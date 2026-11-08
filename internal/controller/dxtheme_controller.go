package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dxv1 "github.com/europesip/hcl-dx-scriptapp-operator/api/v1alpha1"
	"github.com/europesip/hcl-dx-scriptapp-operator/internal/dx"
)

// DXThemeReconciler keeps DX themes: a copy of a base theme with the files of a source, uploaded over WebDAV and
// registered with XMLAccess.
type DXThemeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	*Shared
}

// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxthemes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxthemes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxthemes/finalizers,verbs=update

func themeFolder(t *dxv1.DXTheme) string {
	if t.Spec.Folder != "" {
		return t.Spec.Folder
	}
	return t.Name
}

func themeUniqueName(t *dxv1.DXTheme) string {
	if t.Spec.UniqueName != "" {
		return t.Spec.UniqueName
	}
	return fmt.Sprintf("dxop.%s.%s", t.Namespace, t.Name)
}

func themeBase(t *dxv1.DXTheme) (string, string) {
	f, u := t.Spec.Base.Folder, t.Spec.Base.UniqueName
	if f == "" {
		f = "Portal8.5"
	}
	if u == "" {
		u = "ibm.portal.85Theme"
	}
	return f, u
}

// Reconcile uploads and registers the theme when it changes, and restores it when it is deleted in DX.
func (r *DXThemeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	var t dxv1.DXTheme
	if err := r.Get(ctx, req.NamespacedName, &t); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !t.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&t, Finalizer) {
			return ctrl.Result{}, nil
		}
		if t.Spec.DeletionPolicy == dxv1.DeletionDelete && t.Status.AppliedHash != "" {
			conn, err := r.Connection(ctx, r.Client, t.Namespace, t.Spec.ConnectionRef.Name)
			if err != nil {
				log.Info("cannot delete the theme in DX yet", "reason", err.Error())
				return ctrl.Result{RequeueAfter: waitShort}, nil
			}
			out, err := conn.Client.XMLAccess(ctx, dx.ThemeDeleteXML(themeUniqueName(&t)))
			if err == nil {
				if ok, msg := dx.XMLAccessOK(out); !ok && !containsNotFound(out) {
					err = fmt.Errorf("XMLAccess: %s", msg)
				}
			}
			if err == nil {
				err = conn.Client.DeleteThemeFolder(ctx, themeFolder(&t))
			}
			if err != nil {
				r.Recorder.Eventf(&t, nil, corev1.EventTypeWarning, "DeleteFailed", "Delete", "%v", err)
				return ctrl.Result{RequeueAfter: retryFailed}, nil
			}
			r.Recorder.Eventf(&t, nil, corev1.EventTypeNormal, "Deleted", "Delete", "theme %s deleted from DX", themeUniqueName(&t))
		}
		controllerutil.RemoveFinalizer(&t, Finalizer)
		return ctrl.Result{}, r.Update(ctx, &t)
	}
	if controllerutil.AddFinalizer(&t, Finalizer) {
		if err := r.Update(ctx, &t); err != nil {
			return ctrl.Result{}, err
		}
	}
	orig := t.DeepCopy()
	if t.Spec.Suspend {
		setCondition(&t.Status.Conditions, t.Generation, dxv1.ConditionReady, false, "Suspended", "reconciliation is suspended (spec.suspend)")
		return ctrl.Result{}, r.Status().Patch(ctx, &t, client.MergeFrom(orig))
	}
	res, err := r.reconcile(ctx, &t)
	t.Status.ObservedGeneration = t.Generation
	if uerr := r.Status().Patch(ctx, &t, client.MergeFrom(orig)); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return res, err
}

func containsNotFound(xml string) bool { return strings.Contains(xml, "EJPXA0240E") }

func (r *DXThemeReconciler) reconcile(ctx context.Context, t *dxv1.DXTheme) (ctrl.Result, error) {
	gen := t.Generation
	fail := func(reason string, err error, after time.Duration) (ctrl.Result, error) {
		setCondition(&t.Status.Conditions, gen, dxv1.ConditionReady, false, reason, err.Error())
		if reason != "Waiting" {
			r.Recorder.Eventf(t, nil, corev1.EventTypeWarning, reason, "Reconcile", "%v", err)
		}
		return ctrl.Result{RequeueAfter: after}, nil
	}
	if err := Dependencies(ctx, r.Client, t.Namespace, t.Spec.DependsOn); err != nil {
		if isNotReady(err) {
			setCondition(&t.Status.Conditions, gen, dxv1.ConditionDependenciesReady, false, "Waiting", err.Error())
			return fail("Waiting", err, waitShort)
		}
		return ctrl.Result{}, err
	}
	setCondition(&t.Status.Conditions, gen, dxv1.ConditionDependenciesReady, true, "Ready", "all dependencies are ready")
	conn, err := r.Connection(ctx, r.Client, t.Namespace, t.Spec.ConnectionRef.Name)
	if err != nil {
		if isNotReady(err) {
			return fail("Waiting", err, waitShort)
		}
		return ctrl.Result{}, err
	}
	folder, un := themeFolder(t), themeUniqueName(t)
	baseFolder, baseUN := themeBase(t)
	rev := ""
	var srcDir string
	if t.Spec.Source != nil {
		tree, err := r.Fetcher.Fetch(ctx, t.Namespace, *t.Spec.Source)
		if err != nil {
			return fail("SourceError", err, retryFailed)
		}
		rev, srcDir = tree.Revision, tree.Dir
	}
	spec, _ := json.Marshal(struct {
		F, U, BF, BU, H, D string
		T                  map[string]string
	}{folder, un, baseFolder, baseUN, t.Spec.HeadHTML, t.Spec.Description, t.Spec.Titles})
	hash := hashOf(string(spec), rev, forceToken(t))

	// What is in DX: the registration (and its dxop.hash) and the WebDAV folder
	out, err := conn.Client.XMLAccess(ctx, dx.ThemeExportXML(un))
	if err != nil {
		return fail("DXError", err, retryFailed)
	}
	exp, err := dx.ParseThemeExport(out, un)
	if err != nil {
		return fail("DXError", err, retryFailed)
	}
	hasFolder, err := conn.Client.ThemeFolderExists(ctx, folder)
	if err != nil {
		return fail("DXError", err, retryFailed)
	}
	// The hash kept in DX is what counts: a reconcile that still sees the old object (empty status) after the theme was
	// just registered must not upload it again.
	inSync := exp.Exists && hasFolder && exp.Hash == hash
	reason, drift := "Created", ""
	switch {
	case inSync:
		setCondition(&t.Status.Conditions, gen, dxv1.ConditionDrifted, false, "InSync", "DX matches the resource")
		setCondition(&t.Status.Conditions, gen, dxv1.ConditionReady, true, "InSync", fmt.Sprintf("theme %s is in DX (dav:fs-type1/themes/%s/)", un, folder))
		t.Status.UniqueName, t.Status.AppliedHash = un, hash
		return ctrl.Result{RequeueAfter: conn.Resync()}, nil
	case t.Status.AppliedHash != "" && (!exp.Exists || !hasFolder):
		drift = "the theme was deleted in DX"
		if exp.Exists {
			drift = "the WebDAV folder of the theme was deleted in DX"
		}
		reason = "Restored"
	case t.Status.AppliedHash != "":
		reason = "Updated"
	}
	if drift != "" && t.Spec.DriftPolicy == dxv1.DriftReport {
		setCondition(&t.Status.Conditions, gen, dxv1.ConditionDrifted, true, "Drifted", drift)
		setCondition(&t.Status.Conditions, gen, dxv1.ConditionReady, false, "Drifted", drift)
		r.Recorder.Eventf(t, nil, corev1.EventTypeWarning, "Drifted", "Check", "%s (driftPolicy Report: not restored)", drift)
		return ctrl.Result{RequeueAfter: conn.Resync()}, nil
	}
	if drift != "" {
		r.Recorder.Eventf(t, nil, corev1.EventTypeWarning, "Drifted", "Check", "%s: restoring it", drift)
	}

	// 1. The files: the base theme, then the source on top, then the head HTML
	work := filepath.Join(r.WorkDir, "themes", t.Namespace+"-"+t.Name)
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return ctrl.Result{}, err
	}
	defer os.RemoveAll(work) //nolint:errcheck
	if _, err := conn.Client.DownloadTheme(ctx, baseFolder, work); err != nil {
		return fail("BaseThemeError", fmt.Errorf("copy of the base theme %s: %w", baseFolder, err), retryFailed)
	}
	if srcDir != "" {
		if err := copyTree(srcDir, work); err != nil {
			return ctrl.Result{}, err
		}
	}
	if t.Spec.HeadHTML != "" {
		n, err := dx.AddToHead(work, t.Spec.HeadHTML)
		if err != nil {
			return ctrl.Result{}, err
		}
		if n == 0 {
			return fail("BaseThemeError", fmt.Errorf("no theme*.html with </head> in the base theme %s", baseFolder), retryFailed)
		}
	}
	// 2. Upload, then register (copy of the base registration)
	files, err := conn.Client.UploadTheme(ctx, folder, work)
	if err != nil {
		return fail("UploadFailed", err, retryFailed)
	}
	baseOut, err := conn.Client.XMLAccess(ctx, dx.ThemeExportXML(baseUN))
	if err != nil {
		return fail("DXError", err, retryFailed)
	}
	if ok, msg := dx.XMLAccessOK(baseOut); !ok {
		return fail("BaseThemeError", fmt.Errorf("export of the base theme %s: %s", baseUN, msg), retryFailed)
	}
	reg, err := dx.ThemeRegisterXML(baseOut, dx.ThemeRegistration{UniqueName: un, Folder: folder, BaseUniqueName: baseUN, BaseFolder: baseFolder,
		Titles: t.Spec.Titles, Description: t.Spec.Description, Hash: hash})
	if err != nil {
		return fail("BaseThemeError", err, retryFailed)
	}
	res, err := conn.Client.XMLAccess(ctx, reg)
	if err != nil {
		return fail("DXError", err, retryFailed)
	}
	if ok, msg := dx.XMLAccessOK(res); !ok {
		return fail("RegisterFailed", fmt.Errorf("XMLAccess registration of %s: %s", un, msg), retryFailed)
	}
	t.Status.UniqueName, t.Status.AppliedHash, t.Status.SourceRevision, t.Status.Files, t.Status.LastApplyTime = un, hash, rev, files, now()
	r.Recorder.Eventf(t, nil, corev1.EventTypeNormal, reason, "Apply", "theme %s: %d files in dav:fs-type1/themes/%s/, registered", un, files, folder)
	setCondition(&t.Status.Conditions, gen, dxv1.ConditionDrifted, false, "InSync", "DX matches the resource")
	setCondition(&t.Status.Conditions, gen, dxv1.ConditionReady, true, reason, fmt.Sprintf("theme %s is in DX (dav:fs-type1/themes/%s/)", un, folder))
	return ctrl.Result{RequeueAfter: conn.Resync()}, nil
}

// copyTree copies the files of src over dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func themeRefs(list *dxv1.DXThemeList) map[client.ObjectKey][]refersTo {
	out := map[client.ObjectKey][]refersTo{}
	for i := range list.Items {
		t := &list.Items[i]
		rs := []refersTo{{"DXConnection", t.Spec.ConnectionRef.Name}}
		if t.Spec.Source != nil && t.Spec.Source.ConfigMap != nil {
			rs = append(rs, refersTo{"ConfigMap", t.Spec.Source.ConfigMap.Name})
		}
		for _, d := range t.Spec.DependsOn {
			rs = append(rs, refersTo{d.Kind, d.Name})
		}
		out[client.ObjectKeyFromObject(t)] = rs
	}
	return out
}

// SetupWithManager sets up the controller.
func (r *DXThemeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	newList := func() *dxv1.DXThemeList { return &dxv1.DXThemeList{} }
	m := func(kind string) handler.EventHandler {
		return handler.EnqueueRequestsFromMapFunc(mapReferrers(r.Client, newList, themeRefs, kind))
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dxv1.DXTheme{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}))).
		Watches(&corev1.ConfigMap{}, m("ConfigMap")).
		Watches(&dxv1.DXConnection{}, m("DXConnection")).
		Named("dxtheme").
		Complete(r)
}
