package controller

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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
	"github.com/europesip/hcl-dx-scriptapp-operator/internal/source"
)

// DXScriptApplicationReconciler keeps Script Applications in DX as their resources declare.
type DXScriptApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	*Shared
}

// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxscriptapplications,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxscriptapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxscriptapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch

// wcmDefaults fills the WCM settings that have defaults.
func wcmDefaults(sa *dxv1.DXScriptApplication) dxv1.WCMSettings {
	w := sa.Spec.WCM
	if w.Name == "" {
		w.Name = sa.Name
	}
	if w.Title == "" {
		w.Title = w.Name
	}
	if w.SiteArea == "" {
		w.SiteArea = "Script Application Library/Script Applications/"
	}
	if w.MainHTMLFile == "" {
		w.MainHTMLFile = "index.html"
	}
	return w
}

// Reconcile pushes the application when it changed, checks it in DX on every resync and corrects drift.
func (r *DXScriptApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	var sa dxv1.DXScriptApplication
	if err := r.Get(ctx, req.NamespacedName, &sa); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deletion: Retain leaves the application in DX; Delete removes it
	if !sa.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&sa, Finalizer) {
			return ctrl.Result{}, nil
		}
		if sa.Spec.DeletionPolicy == dxv1.DeletionDelete && sa.Spec.Mode != dxv1.ModeVerify && sa.Status.ContentID != "" {
			conn, err := r.Connection(ctx, r.Client, sa.Namespace, sa.Spec.ConnectionRef.Name)
			if err != nil {
				log.Info("cannot remove the application from DX yet", "reason", err.Error())
				return ctrl.Result{RequeueAfter: waitShort}, nil
			}
			if err := conn.Client.DeleteContent(ctx, sa.Status.ContentID); err != nil {
				r.Recorder.Eventf(&sa, nil, corev1.EventTypeWarning, "UndeployFailed", "Delete", "%v", err)
				return ctrl.Result{RequeueAfter: retryFailed}, nil
			}
			r.Recorder.Eventf(&sa, nil, corev1.EventTypeNormal, "Undeployed", "Delete", "removed %s from DX", sa.Status.ContentID)
		}
		controllerutil.RemoveFinalizer(&sa, Finalizer)
		return ctrl.Result{}, r.Update(ctx, &sa)
	}
	if controllerutil.AddFinalizer(&sa, Finalizer) {
		if err := r.Update(ctx, &sa); err != nil {
			return ctrl.Result{}, err
		}
	}
	orig := sa.DeepCopy()
	if sa.Spec.Suspend {
		setCondition(&sa.Status.Conditions, sa.Generation, dxv1.ConditionReady, false, "Suspended", "reconciliation is suspended (spec.suspend)")
		return ctrl.Result{}, r.Status().Patch(ctx, &sa, client.MergeFrom(orig))
	}

	res, err := r.reconcile(ctx, &sa)
	sa.Status.ObservedGeneration = sa.Generation
	if uerr := r.Status().Patch(ctx, &sa, client.MergeFrom(orig)); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return res, err
}

func (r *DXScriptApplicationReconciler) reconcile(ctx context.Context, sa *dxv1.DXScriptApplication) (ctrl.Result, error) {
	gen := sa.Generation
	fail := func(reason string, err error, after time.Duration) (ctrl.Result, error) {
		setCondition(&sa.Status.Conditions, gen, dxv1.ConditionReady, false, reason, err.Error())
		if reason != "Waiting" {
			r.Recorder.Eventf(sa, nil, corev1.EventTypeWarning, reason, "Reconcile", "%v", err)
		}
		return ctrl.Result{RequeueAfter: after}, nil
	}

	// 1. Dependencies: the explicit ones and the DAM collections used in substitutions
	deps := append([]dxv1.Dependency{}, sa.Spec.DependsOn...)
	for _, s := range sa.Spec.Substitutions {
		if s.ValueFrom != nil && s.ValueFrom.DAMAsset != nil {
			deps = append(deps, dxv1.Dependency{Kind: "DXDamCollection", Name: s.ValueFrom.DAMAsset.Collection})
		}
	}
	if err := Dependencies(ctx, r.Client, sa.Namespace, deps); err != nil {
		if isNotReady(err) {
			setCondition(&sa.Status.Conditions, gen, dxv1.ConditionDependenciesReady, false, "Waiting", err.Error())
			return fail("Waiting", err, waitShort)
		}
		return ctrl.Result{}, err
	}
	setCondition(&sa.Status.Conditions, gen, dxv1.ConditionDependenciesReady, true, "Ready", "all dependencies are ready")

	// 2. Connection
	conn, err := r.Connection(ctx, r.Client, sa.Namespace, sa.Spec.ConnectionRef.Name)
	if err != nil {
		if isNotReady(err) {
			return fail("Waiting", err, waitShort)
		}
		return ctrl.Result{}, err
	}
	w := wcmDefaults(sa)
	sa.Status.LastCheckTime = now()

	// 3. Verify mode: only check that the application exists in DX
	if sa.Spec.Mode == dxv1.ModeVerify {
		c, err := conn.Client.FindContentByName(ctx, w.Name)
		if err != nil {
			return fail("DXError", err, retryFailed)
		}
		if c == nil {
			return fail("NotFound", fmt.Errorf("the Script Application %q is not in DX (mode Verify: the operator does not push it)", w.Name), conn.Resync())
		}
		sa.Status.ContentID, sa.Status.WCMLastModified = c.ID, c.Updated
		setCondition(&sa.Status.Conditions, gen, dxv1.ConditionReady, true, "Verified", fmt.Sprintf("%q is in DX (content ID %s)", w.Name, c.ID))
		return ctrl.Result{RequeueAfter: conn.Resync()}, nil
	}

	// 4. Source and substitutions
	subs, err := r.substitutions(ctx, sa)
	if err != nil {
		if isNotReady(err) {
			return fail("Waiting", err, waitShort)
		}
		return fail("SubstitutionError", err, retryFailed)
	}
	tree, err := r.Fetcher.Fetch(ctx, sa.Namespace, sa.Spec.Source)
	if err != nil {
		return fail("SourceError", err, retryFailed)
	}
	work := filepath.Join(r.WorkDir, "apps", sa.Namespace, sa.Name)
	prep, err := source.Prepare(tree, work, w.MainHTMLFile, subs, w.Name, w.Title, w.SiteArea, w.MainHTMLFile, w.ProjectContext, forceToken(sa))
	if err != nil {
		return fail("InvalidSource", err, retryFailed)
	}

	// 5. Push when something changed, or when the application was deleted or edited in DX
	reason := ""
	switch {
	case sa.Status.ContentID == "":
		reason = "Created"
	case sa.Status.AppliedHash != prep.Hash:
		reason = "Updated"
	default:
		c, err := conn.Client.GetContent(ctx, sa.Status.ContentID)
		drift := ""
		switch {
		case errors.Is(err, dx.ErrNotFound):
			drift = "the application was deleted in DX"
		case err != nil:
			return fail("DXError", err, retryFailed)
		case sa.Status.WCMLastModified != "" && c.Updated != sa.Status.WCMLastModified:
			drift = fmt.Sprintf("the application was modified in DX (%s)", c.Updated)
		}
		if drift != "" {
			if sa.Spec.DriftPolicy == dxv1.DriftReport {
				setCondition(&sa.Status.Conditions, gen, dxv1.ConditionDrifted, true, "Drifted", drift)
				r.Recorder.Eventf(sa, nil, corev1.EventTypeWarning, "Drifted", "Check", "%s (driftPolicy Report: not restored)", drift)
				setCondition(&sa.Status.Conditions, gen, dxv1.ConditionReady, false, "Drifted", drift)
				return ctrl.Result{RequeueAfter: conn.Resync()}, nil
			}
			r.Recorder.Eventf(sa, nil, corev1.EventTypeWarning, "Drifted", "Check", "%s: restoring it", drift)
			reason = "Restored"
			if errors.Is(err, dx.ErrNotFound) {
				sa.Status.ContentID = "" // a new item will be created
			}
		}
	}
	if reason == "" {
		setCondition(&sa.Status.Conditions, gen, dxv1.ConditionDrifted, false, "InSync", "DX matches the resource")
		setCondition(&sa.Status.Conditions, gen, dxv1.ConditionReady, true, "InSync", fmt.Sprintf("%q is in DX (content ID %s)", w.Name, sa.Status.ContentID))
		return ctrl.Result{RequeueAfter: conn.Resync()}, nil
	}

	id, _, err := r.DXClient.Push(ctx, conn.Config, dx.PushRequest{SiteArea: w.SiteArea, Name: w.Name, Title: w.Title,
		Dir: prep.Dir, MainHTMLFile: w.MainHTMLFile, ProjectContext: w.ProjectContext})
	if err != nil {
		return fail("PushFailed", err, retryFailed)
	}
	sa.Status.ContentID = id
	if c, err := conn.Client.GetContent(ctx, id); err == nil {
		sa.Status.WCMLastModified = c.Updated
	} else {
		sa.Status.WCMLastModified = ""
	}
	sa.Status.AppliedHash, sa.Status.SourceRevision = prep.Hash, tree.Revision
	sa.Status.Files, sa.Status.Size = prep.Files, source.HumanSize(prep.Bytes)
	sa.Status.LastPushTime = now()
	sa.Status.History = append([]dxv1.PushRecord{{Time: *sa.Status.LastPushTime, SourceRevision: tree.Revision, Reason: reason}}, sa.Status.History...)
	if len(sa.Status.History) > 10 {
		sa.Status.History = sa.Status.History[:10]
	}
	r.Recorder.Eventf(sa, nil, corev1.EventTypeNormal, "Pushed", "Push", "%s: %q pushed to DX (content ID %s, %d files, %s)", reason, w.Name, id, prep.Files, sa.Status.Size)
	setCondition(&sa.Status.Conditions, gen, dxv1.ConditionDrifted, false, "InSync", "DX matches the resource")
	setCondition(&sa.Status.Conditions, gen, dxv1.ConditionReady, true, reason, fmt.Sprintf("%q is in DX (content ID %s)", w.Name, id))
	return ctrl.Result{RequeueAfter: conn.Resync()}, nil
}

// substitutions resolves the values of the ${NAME} placeholders.
func (r *DXScriptApplicationReconciler) substitutions(ctx context.Context, sa *dxv1.DXScriptApplication) (map[string]string, error) {
	out := map[string]string{}
	for _, s := range sa.Spec.Substitutions {
		v := s.Value
		if f := s.ValueFrom; f != nil {
			switch {
			case f.ConfigMapKeyRef != nil:
				var cm corev1.ConfigMap
				if err := r.Get(ctx, client.ObjectKey{Namespace: sa.Namespace, Name: f.ConfigMapKeyRef.Name}, &cm); err != nil {
					return nil, fmt.Errorf("substitution %s: ConfigMap %s: %w", s.Name, f.ConfigMapKeyRef.Name, err)
				}
				var ok bool
				if v, ok = cm.Data[f.ConfigMapKeyRef.Key]; !ok {
					return nil, fmt.Errorf("substitution %s: ConfigMap %s has no key %s", s.Name, cm.Name, f.ConfigMapKeyRef.Key)
				}
			case f.SecretKeyRef != nil:
				var sec corev1.Secret
				if err := r.Get(ctx, client.ObjectKey{Namespace: sa.Namespace, Name: f.SecretKeyRef.Name}, &sec); err != nil {
					return nil, fmt.Errorf("substitution %s: Secret %s: %w", s.Name, f.SecretKeyRef.Name, err)
				}
				b, ok := sec.Data[f.SecretKeyRef.Key]
				if !ok {
					return nil, fmt.Errorf("substitution %s: Secret %s has no key %s", s.Name, sec.Name, f.SecretKeyRef.Key)
				}
				v = string(b)
			case f.DAMAsset != nil:
				var col dxv1.DXDamCollection
				if err := r.Get(ctx, client.ObjectKey{Namespace: sa.Namespace, Name: f.DAMAsset.Collection}, &col); err != nil {
					return nil, notReady("substitution %s: DXDamCollection %s: %v", s.Name, f.DAMAsset.Collection, err)
				}
				found := false
				for _, a := range col.Status.Assets {
					if a.Name == f.DAMAsset.Asset && a.URL != "" {
						v, found = a.URL, true
					}
				}
				if !found {
					return nil, notReady("substitution %s: the DXDamCollection %s has no asset %q yet", s.Name, col.Name, f.DAMAsset.Asset)
				}
			}
		}
		out[s.Name] = v
	}
	return out, nil
}

// references of a DXScriptApplication, for the watches.
func scriptAppRefs(list *dxv1.DXScriptApplicationList) map[client.ObjectKey][]refersTo {
	out := map[client.ObjectKey][]refersTo{}
	for i := range list.Items {
		sa := &list.Items[i]
		var rs []refersTo
		if sa.Spec.Source.ConfigMap != nil {
			rs = append(rs, refersTo{"ConfigMap", sa.Spec.Source.ConfigMap.Name})
		}
		for _, s := range sa.Spec.Substitutions {
			if f := s.ValueFrom; f != nil {
				if f.ConfigMapKeyRef != nil {
					rs = append(rs, refersTo{"ConfigMap", f.ConfigMapKeyRef.Name})
				}
				if f.SecretKeyRef != nil {
					rs = append(rs, refersTo{"Secret", f.SecretKeyRef.Name})
				}
				if f.DAMAsset != nil {
					rs = append(rs, refersTo{"DXDamCollection", f.DAMAsset.Collection})
				}
			}
		}
		for _, d := range sa.Spec.DependsOn {
			rs = append(rs, refersTo{d.Kind, d.Name})
		}
		rs = append(rs, refersTo{"DXConnection", sa.Spec.ConnectionRef.Name})
		out[client.ObjectKeyFromObject(sa)] = rs
	}
	return out
}

// SetupWithManager sets up the controller: it reacts to its resources and to every object they refer to.
func (r *DXScriptApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	newList := func() *dxv1.DXScriptApplicationList { return &dxv1.DXScriptApplicationList{} }
	m := func(kind string) handler.EventHandler {
		return handler.EnqueueRequestsFromMapFunc(mapReferrers(r.Client, newList, scriptAppRefs, kind))
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dxv1.DXScriptApplication{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}))).
		Watches(&corev1.ConfigMap{}, m("ConfigMap")).
		Watches(&corev1.Secret{}, m("Secret")).
		Watches(&dxv1.DXConnection{}, m("DXConnection")).
		Watches(&dxv1.DXDamCollection{}, m("DXDamCollection")).
		Watches(&dxv1.DXDatabase{}, m("DXDatabase")).
		Watches(&appsv1.Deployment{}, m("Deployment")).
		Watches(&appsv1.StatefulSet{}, m("StatefulSet")).
		Watches(&batchv1.Job{}, m("Job")).
		Named("dxscriptapplication").
		Complete(r)
}
