package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

// DXPageReconciler keeps DX pages with Script Applications as their resources declare (XMLAccess).
type DXPageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	*Shared
}

// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxpages,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxpages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxpages/finalizers,verbs=update

func pageUniqueName(p *dxv1.DXPage) string {
	if p.Spec.UniqueName != "" {
		return p.Spec.UniqueName
	}
	return "dxop." + p.Namespace + "." + p.Name
}

// Reconcile creates or updates the page, checks it on every resync and recreates it if it was deleted.
func (r *DXPageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	var p dxv1.DXPage
	if err := r.Get(ctx, req.NamespacedName, &p); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !p.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&p, Finalizer) {
			return ctrl.Result{}, nil
		}
		if p.Spec.DeletionPolicy == dxv1.DeletionDelete && p.Status.AppliedHash != "" {
			conn, err := r.Connection(ctx, r.Client, p.Namespace, p.Spec.ConnectionRef.Name)
			if err != nil {
				log.Info("cannot delete the page in DX yet", "reason", err.Error())
				return ctrl.Result{RequeueAfter: waitShort}, nil
			}
			res, err := conn.Client.XMLAccess(ctx, dx.PageDeleteXML(pageUniqueName(&p)))
			if ok, msg := dx.XMLAccessOK(res); err != nil || (!ok && !strings.Contains(res, "EJPXA0240E")) {
				r.Recorder.Eventf(&p, nil, corev1.EventTypeWarning, "DeleteFailed", "Delete", "%v %s", err, msg)
				return ctrl.Result{RequeueAfter: retryFailed}, nil
			}
			r.Recorder.Eventf(&p, nil, corev1.EventTypeNormal, "Deleted", "Delete", "page %s deleted from DX", pageUniqueName(&p))
		}
		controllerutil.RemoveFinalizer(&p, Finalizer)
		return ctrl.Result{}, r.Update(ctx, &p)
	}
	if controllerutil.AddFinalizer(&p, Finalizer) {
		if err := r.Update(ctx, &p); err != nil {
			return ctrl.Result{}, err
		}
	}
	orig := p.DeepCopy()
	if p.Spec.Suspend {
		setCondition(&p.Status.Conditions, p.Generation, dxv1.ConditionReady, false, "Suspended", "reconciliation is suspended (spec.suspend)")
		return ctrl.Result{}, r.Status().Patch(ctx, &p, client.MergeFrom(orig))
	}
	res, err := r.reconcile(ctx, &p)
	p.Status.ObservedGeneration = p.Generation
	if uerr := r.Status().Patch(ctx, &p, client.MergeFrom(orig)); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return res, err
}

func (r *DXPageReconciler) reconcile(ctx context.Context, p *dxv1.DXPage) (ctrl.Result, error) {
	gen := p.Generation
	fail := func(reason, msg string, after interface{ String() string }) {
		setCondition(&p.Status.Conditions, gen, dxv1.ConditionReady, false, reason, msg)
		if reason != "Waiting" {
			r.Recorder.Eventf(p, nil, corev1.EventTypeWarning, reason, "Reconcile", "%s", msg)
		}
	}
	// 1. Dependencies: the applications of the page and the explicit ones
	deps := append([]dxv1.Dependency{}, p.Spec.DependsOn...)
	for _, a := range p.Spec.Applications {
		deps = append(deps, dxv1.Dependency{Kind: "ScriptApplication", Name: a.Name})
	}
	if err := Dependencies(ctx, r.Client, p.Namespace, deps); err != nil {
		if isNotReady(err) {
			setCondition(&p.Status.Conditions, gen, dxv1.ConditionDependenciesReady, false, "Waiting", err.Error())
			fail("Waiting", err.Error(), waitShort)
			return ctrl.Result{RequeueAfter: waitShort}, nil
		}
		return ctrl.Result{}, err
	}
	setCondition(&p.Status.Conditions, gen, dxv1.ConditionDependenciesReady, true, "Ready", "all dependencies are ready")
	conn, err := r.Connection(ctx, r.Client, p.Namespace, p.Spec.ConnectionRef.Name)
	if err != nil {
		if isNotReady(err) {
			fail("Waiting", err.Error(), waitShort)
			return ctrl.Result{RequeueAfter: waitShort}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. The page as declared: content IDs of its applications, in order
	var ids []string
	for _, a := range p.Spec.Applications {
		var sa dxv1.ScriptApplication
		if err := r.Get(ctx, client.ObjectKey{Namespace: p.Namespace, Name: a.Name}, &sa); err != nil || sa.Status.ContentID == "" {
			fail("Waiting", fmt.Sprintf("waiting for the content ID of ScriptApplication %s", a.Name), waitShort)
			return ctrl.Result{RequeueAfter: waitShort}, nil
		}
		ids = append(ids, sa.Status.ContentID)
	}
	un := pageUniqueName(p)
	friendly := p.Spec.FriendlyName
	if friendly == "" {
		friendly = p.Name
	}
	parent := p.Spec.Parent
	if parent == "" {
		parent = "ibm.portal.Home"
	}
	auth := p.Spec.Access.Authenticated == nil || *p.Spec.Access.Authenticated
	page := dx.Page{UniqueName: un, Parent: parent, FriendlyName: friendly, Titles: p.Spec.Titles, ContentIDs: ids,
		Ordinal: p.Spec.Ordinal, LayoutTemplate: p.Spec.LayoutTemplate, Skin: p.Spec.Skin, Theme: p.Spec.Theme,
		Anonymous: p.Spec.Access.Anonymous, Authenticated: auth}
	if page.LayoutTemplate == "" {
		page.LayoutTemplate = "dav:fs-type1/themes/Portal8.5/layout-templates/1Column"
	}
	if page.Skin == "" {
		page.Skin = "ibm.portal.85NoSkin"
	}
	locales := make([]string, 0, len(p.Spec.Titles))
	for l, t := range p.Spec.Titles {
		locales = append(locales, l+"="+t)
	}
	sort.Strings(locales)
	layoutHash := hashOf(append([]string{page.Skin}, ids...)...)
	ord := ""
	if page.Ordinal != nil {
		ord = fmt.Sprint(*page.Ordinal)
	}
	page.Hash = hashOf(un, parent, friendly, strings.Join(locales, "|"), page.LayoutTemplate, page.Theme,
		fmt.Sprint(page.Anonymous, auth), ord, layoutHash, forceToken(p))

	// 3. What is in DX
	exp, err := conn.Client.XMLAccess(ctx, dx.PageExportXML(un))
	var cur dx.PageExport
	if err == nil {
		cur, err = dx.ParsePageExport(exp, un)
	}
	if err != nil {
		fail("DXError", err.Error(), retryFailed)
		return ctrl.Result{RequeueAfter: retryFailed}, nil
	}
	p.Status.LastCheckTime = now()
	p.Status.UniqueName = un
	if parent == "ibm.portal.Home" {
		p.Status.URL = conn.Config.BaseURL + "/wps/myportal/Home/" + friendly
	}

	// 4. Decide: create, update (keeping the layout), rebuild the layout, restore, or nothing
	withLayout, reason := false, ""
	switch {
	case !cur.Exists && p.Status.AppliedHash == "":
		withLayout, reason = true, "Created"
	case !cur.Exists:
		if p.Spec.DriftPolicy == dxv1.DriftReport {
			return r.reportDrift(p, conn, "the page was deleted in DX")
		}
		r.Recorder.Eventf(p, nil, corev1.EventTypeWarning, "Drifted", "Check", "the page %s was deleted in DX: recreating it", un)
		withLayout, reason = true, "Restored"
	case p.Status.LayoutHash != "" && p.Status.LayoutHash != layoutHash:
		// Other applications: the viewers are rebuilt. XMLAccess cannot replace the components of a page
		// in place, so the page is deleted and created again (settings made on it are lost).
		if res, err := conn.Client.XMLAccess(ctx, dx.PageDeleteXML(un)); err != nil {
			fail("XMLAccessFailed", err.Error(), retryFailed)
			return ctrl.Result{RequeueAfter: retryFailed}, nil
		} else if ok, msg := dx.XMLAccessOK(res); !ok {
			fail("XMLAccessFailed", "delete before rebuilding the layout: "+msg, retryFailed)
			return ctrl.Result{RequeueAfter: retryFailed}, nil
		}
		withLayout, reason = true, "Rebuilt"
	case cur.Hash != page.Hash && p.Status.AppliedHash == page.Hash:
		if p.Spec.DriftPolicy == dxv1.DriftReport {
			return r.reportDrift(p, conn, "the settings of the page were changed in DX")
		}
		reason = "Restored"
	case cur.Hash != page.Hash:
		reason = "Updated"
	}
	if reason == "" {
		if p.Status.AppliedHash == "" {
			// Adopted: the page is already in DX exactly as declared (applied before by this operator, e.g. by an
			// earlier resource with the same spec). Record it, so deletionPolicy and layout changes apply to it.
			p.Status.AppliedHash, p.Status.LayoutHash, p.Status.LastApplyTime = page.Hash, layoutHash, now()
			r.Recorder.Eventf(p, nil, corev1.EventTypeNormal, "Adopted", "Apply", "page %s adopted (already in DX as declared)", un)
		}
		setCondition(&p.Status.Conditions, gen, dxv1.ConditionDrifted, false, "InSync", "DX matches the resource")
		setCondition(&p.Status.Conditions, gen, dxv1.ConditionReady, true, "InSync", "page "+un+" is in DX")
		return ctrl.Result{RequeueAfter: conn.Resync()}, nil
	}
	res, err := conn.Client.XMLAccess(ctx, dx.PageUpdateXML(page, withLayout))
	if err == nil {
		if ok, msg := dx.XMLAccessOK(res); !ok {
			err = fmt.Errorf("%s", msg)
		}
	}
	if err != nil {
		fail("XMLAccessFailed", fmt.Sprintf("XMLAccess could not apply the page %s: %v", un, err), retryFailed)
		return ctrl.Result{RequeueAfter: retryFailed}, nil
	}
	p.Status.AppliedHash, p.Status.LayoutHash, p.Status.LastApplyTime = page.Hash, layoutHash, now()
	r.Recorder.Eventf(p, nil, corev1.EventTypeNormal, reason, "Apply", "page %s %s in DX", un, strings.ToLower(reason))
	setCondition(&p.Status.Conditions, gen, dxv1.ConditionDrifted, false, "InSync", "DX matches the resource")
	setCondition(&p.Status.Conditions, gen, dxv1.ConditionReady, true, reason, "page "+un+" is in DX")
	return ctrl.Result{RequeueAfter: conn.Resync()}, nil
}

func (r *DXPageReconciler) reportDrift(p *dxv1.DXPage, conn *Connection, msg string) (ctrl.Result, error) {
	setCondition(&p.Status.Conditions, p.Generation, dxv1.ConditionDrifted, true, "Drifted", msg)
	setCondition(&p.Status.Conditions, p.Generation, dxv1.ConditionReady, false, "Drifted", msg)
	r.Recorder.Eventf(p, nil, corev1.EventTypeWarning, "Drifted", "Check", "%s (driftPolicy Report: not restored)", msg)
	return ctrl.Result{RequeueAfter: conn.Resync()}, nil
}

func pageRefs(list *dxv1.DXPageList) map[client.ObjectKey][]refersTo {
	out := map[client.ObjectKey][]refersTo{}
	for i := range list.Items {
		p := &list.Items[i]
		rs := []refersTo{{"DXConnection", p.Spec.ConnectionRef.Name}}
		for _, a := range p.Spec.Applications {
			rs = append(rs, refersTo{"ScriptApplication", a.Name})
		}
		for _, d := range p.Spec.DependsOn {
			rs = append(rs, refersTo{d.Kind, d.Name})
		}
		out[client.ObjectKeyFromObject(p)] = rs
	}
	return out
}

// SetupWithManager sets up the controller: a new content ID of one of its applications re-applies the page.
func (r *DXPageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	newList := func() *dxv1.DXPageList { return &dxv1.DXPageList{} }
	m := func(kind string) handler.EventHandler {
		return handler.EnqueueRequestsFromMapFunc(mapReferrers(r.Client, newList, pageRefs, kind))
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dxv1.DXPage{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}))).
		Watches(&dxv1.ScriptApplication{}, m("ScriptApplication")).
		Watches(&dxv1.DXConnection{}, m("DXConnection")).
		Named("dxpage").
		Complete(r)
}
