package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	"github.com/europesip/hcl-dx-scriptapp-operator/internal/source"
)

// DAMCollectionReconciler keeps DAM collections with the files of their source.
type DAMCollectionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	*Shared
}

// +kubebuilder:rbac:groups=dx.europesip.com,resources=damcollections,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=damcollections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=damcollections/finalizers,verbs=update

func collectionName(c *dxv1.DAMCollection) string {
	if c.Spec.CollectionName != "" {
		return c.Spec.CollectionName
	}
	return c.Name
}

// Reconcile creates the collection, uploads new and changed files (a changed file becomes a new version of
// the same asset, so its URL does not change), and restores what was deleted in DX.
func (r *DAMCollectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	var c dxv1.DAMCollection
	if err := r.Get(ctx, req.NamespacedName, &c); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !c.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&c, Finalizer) {
			return ctrl.Result{}, nil
		}
		if c.Spec.DeletionPolicy == dxv1.DeletionDelete && c.Status.CollectionID != "" {
			conn, err := r.Connection(ctx, r.Client, c.Namespace, c.Spec.ConnectionRef.Name)
			if err != nil {
				log.Info("cannot delete the collection in DX yet", "reason", err.Error())
				return ctrl.Result{RequeueAfter: waitShort}, nil
			}
			if err := conn.Client.DeleteCollection(ctx, c.Status.CollectionID); err != nil {
				r.Recorder.Eventf(&c, nil, corev1.EventTypeWarning, "DeleteFailed", "Delete", "%v", err)
				return ctrl.Result{RequeueAfter: retryFailed}, nil
			}
			r.Recorder.Eventf(&c, nil, corev1.EventTypeNormal, "Deleted", "Delete", "DAM collection %s deleted", collectionName(&c))
		}
		controllerutil.RemoveFinalizer(&c, Finalizer)
		return ctrl.Result{}, r.Update(ctx, &c)
	}
	if controllerutil.AddFinalizer(&c, Finalizer) {
		if err := r.Update(ctx, &c); err != nil {
			return ctrl.Result{}, err
		}
	}
	orig := c.DeepCopy()
	if c.Spec.Suspend {
		setCondition(&c.Status.Conditions, c.Generation, dxv1.ConditionReady, false, "Suspended", "reconciliation is suspended (spec.suspend)")
		return ctrl.Result{}, r.Status().Patch(ctx, &c, client.MergeFrom(orig))
	}
	res, err := r.reconcile(ctx, &c)
	c.Status.ObservedGeneration = c.Generation
	if uerr := r.Status().Patch(ctx, &c, client.MergeFrom(orig)); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return res, err
}

func (r *DAMCollectionReconciler) reconcile(ctx context.Context, c *dxv1.DAMCollection) (ctrl.Result, error) {
	gen := c.Generation
	fail := func(reason string, err error, after time.Duration) (ctrl.Result, error) {
		setCondition(&c.Status.Conditions, gen, dxv1.ConditionReady, false, reason, err.Error())
		if reason != "Waiting" {
			r.Recorder.Eventf(c, nil, corev1.EventTypeWarning, reason, "Reconcile", "%v", err)
		}
		return ctrl.Result{RequeueAfter: after}, nil
	}
	if err := Dependencies(ctx, r.Client, c.Namespace, c.Spec.DependsOn); err != nil {
		if isNotReady(err) {
			setCondition(&c.Status.Conditions, gen, dxv1.ConditionDependenciesReady, false, "Waiting", err.Error())
			return fail("Waiting", err, waitShort)
		}
		return ctrl.Result{}, err
	}
	setCondition(&c.Status.Conditions, gen, dxv1.ConditionDependenciesReady, true, "Ready", "all dependencies are ready")
	conn, err := r.Connection(ctx, r.Client, c.Namespace, c.Spec.ConnectionRef.Name)
	if err != nil {
		if isNotReady(err) {
			return fail("Waiting", err, waitShort)
		}
		return ctrl.Result{}, err
	}
	tree, err := r.Fetcher.Fetch(ctx, c.Namespace, c.Spec.Source)
	if err != nil {
		return fail("SourceError", err, retryFailed)
	}
	name := collectionName(c)
	drifts := []string{}

	// 1. The collection: the one in the status, or found by name, or a new one
	var col *dx.Collection
	if c.Status.CollectionID != "" {
		col, err = conn.Client.GetCollection(ctx, c.Status.CollectionID)
		if errors.Is(err, dx.ErrNotFound) {
			col, err = nil, nil
			drifts = append(drifts, "the collection was deleted in DX")
		}
		if err != nil {
			return fail("DXError", err, retryFailed)
		}
	}
	if col == nil {
		if col, err = conn.Client.FindCollection(ctx, name); err != nil {
			return fail("DXError", err, retryFailed)
		}
	}
	if len(drifts) > 0 && c.Spec.DriftPolicy == dxv1.DriftReport {
		return r.reportDrift(c, conn, drifts[0])
	}
	if col == nil {
		if col, err = conn.Client.CreateCollection(ctx, name, c.Spec.Description); err != nil {
			return fail("DXError", err, retryFailed)
		}
		r.Recorder.Eventf(c, nil, corev1.EventTypeNormal, "Created", "Create", "DAM collection %s created", name)
		c.Status.Assets = nil
	}
	c.Status.CollectionID = col.ID
	auth := c.Spec.Access.Authenticated == nil || *c.Spec.Access.Authenticated
	conn.Client.SetCollectionAccess(ctx, col, c.Spec.Access.Anonymous, auth)

	// 2. Assets: compare the source with the status and with what is in the DAM
	inDAM, err := conn.Client.ListAssets(ctx, col.ID)
	if err != nil {
		return fail("DXError", err, retryFailed)
	}
	byID := map[string]dx.Asset{}
	byName := map[string]dx.Asset{}
	for _, a := range inDAM {
		byID[a.ID], byName[a.Name] = a, a
	}
	prev := map[string]dxv1.DAMAssetStatus{}
	for _, a := range c.Status.Assets {
		prev[a.Name] = a
	}
	var assets []dxv1.DAMAssetStatus
	uploaded, replaced := 0, 0
	for _, f := range tree.Files {
		an := source.AssetName(f.Path)
		data, err := os.ReadFile(filepath.Join(tree.Dir, filepath.FromSlash(f.Path)))
		if err != nil {
			return ctrl.Result{}, err
		}
		st, known := prev[an]
		_, exists := byID[st.ID]
		switch {
		case known && exists && st.SHA256 == f.SHA256:
			// unchanged
		case known && exists:
			if err := conn.Client.ReplaceAsset(ctx, col.ID, st.ID, an, data); err != nil {
				return fail("UploadFailed", err, retryFailed)
			}
			st.SHA256 = f.SHA256
			replaced++
		default:
			if known && !exists {
				drifts = append(drifts, fmt.Sprintf("asset %s was deleted in DX", an))
				if c.Spec.DriftPolicy == dxv1.DriftReport {
					return r.reportDrift(c, conn, drifts[len(drifts)-1])
				}
			}
			if a, ok := byName[an]; ok { // adopt an asset with that name (for example after the status was lost)
				if err := conn.Client.ReplaceAsset(ctx, col.ID, a.ID, an, data); err != nil {
					return fail("UploadFailed", err, retryFailed)
				}
				st = dxv1.DAMAssetStatus{Name: an, ID: a.ID, SHA256: f.SHA256}
				replaced++
			} else {
				a, err := conn.Client.UploadAsset(ctx, col.ID, an, data)
				if err != nil {
					return fail("UploadFailed", err, retryFailed)
				}
				st = dxv1.DAMAssetStatus{Name: an, ID: a.ID, SHA256: f.SHA256}
				uploaded++
			}
			st.URL = ""
		}
		if st.URL == "" {
			if u, err := conn.Client.AssetURL(ctx, col.ID, st.ID, 2*time.Minute); err == nil {
				st.URL = u
			} else {
				return fail("RenditionPending", fmt.Errorf("asset %s: %w", an, err), waitShort)
			}
		}
		assets = append(assets, st)
		delete(prev, an)
	}
	// 3. Prune: assets that left the source
	pruned := 0
	if c.Spec.Prune {
		for _, a := range prev {
			if err := conn.Client.DeleteAsset(ctx, col.ID, a.ID); err != nil {
				return fail("DXError", err, retryFailed)
			}
			pruned++
		}
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
	c.Status.Assets, c.Status.SourceRevision, c.Status.LastSyncTime = assets, tree.Revision, now()
	if len(drifts) > 0 {
		r.Recorder.Eventf(c, nil, corev1.EventTypeWarning, "Drifted", "Check", "%s: restored", drifts[0])
	}
	if uploaded+replaced+pruned > 0 {
		r.Recorder.Eventf(c, nil, corev1.EventTypeNormal, "Synced", "Upload", "DAM collection %s: %d uploaded, %d new versions, %d deleted", name, uploaded, replaced, pruned)
	}
	setCondition(&c.Status.Conditions, gen, dxv1.ConditionDrifted, false, "InSync", "DX matches the resource")
	setCondition(&c.Status.Conditions, gen, dxv1.ConditionReady, true, "InSync", fmt.Sprintf("collection %s with %d assets", name, len(assets)))
	return ctrl.Result{RequeueAfter: conn.Resync()}, nil
}

func (r *DAMCollectionReconciler) reportDrift(c *dxv1.DAMCollection, conn *Connection, msg string) (ctrl.Result, error) {
	setCondition(&c.Status.Conditions, c.Generation, dxv1.ConditionDrifted, true, "Drifted", msg)
	setCondition(&c.Status.Conditions, c.Generation, dxv1.ConditionReady, false, "Drifted", msg)
	r.Recorder.Eventf(c, nil, corev1.EventTypeWarning, "Drifted", "Check", "%s (driftPolicy Report: not restored)", msg)
	return ctrl.Result{RequeueAfter: conn.Resync()}, nil
}

func damRefs(list *dxv1.DAMCollectionList) map[client.ObjectKey][]refersTo {
	out := map[client.ObjectKey][]refersTo{}
	for i := range list.Items {
		c := &list.Items[i]
		rs := []refersTo{{"DXConnection", c.Spec.ConnectionRef.Name}}
		if c.Spec.Source.ConfigMap != nil {
			rs = append(rs, refersTo{"ConfigMap", c.Spec.Source.ConfigMap.Name})
		}
		for _, d := range c.Spec.DependsOn {
			rs = append(rs, refersTo{d.Kind, d.Name})
		}
		out[client.ObjectKeyFromObject(c)] = rs
	}
	return out
}

// SetupWithManager sets up the controller.
func (r *DAMCollectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	newList := func() *dxv1.DAMCollectionList { return &dxv1.DAMCollectionList{} }
	m := func(kind string) handler.EventHandler {
		return handler.EnqueueRequestsFromMapFunc(mapReferrers(r.Client, newList, damRefs, kind))
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dxv1.DAMCollection{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}))).
		Watches(&corev1.ConfigMap{}, m("ConfigMap")).
		Watches(&dxv1.DXConnection{}, m("DXConnection")).
		Named("damcollection").
		Complete(r)
}
