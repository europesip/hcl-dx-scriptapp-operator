package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dxv1 "github.com/europesip/hcl-dx-scriptapp-operator/api/v1alpha1"
	"github.com/europesip/hcl-dx-scriptapp-operator/internal/dx"
)

// DXConnectionReconciler checks that DX answers and accepts the credentials.
type DXConnectionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	*Shared
}

// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile signs in to DX with the connection, and reports the result in the Ready condition.
func (r *DXConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	var conn dxv1.DXConnection
	if err := r.Get(ctx, req.NamespacedName, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	before := conn.Status.DeepCopy()
	orig := conn.DeepCopy()
	resync := (&Connection{Object: &conn}).Resync()

	ok, reason, msg := r.check(ctx, &conn)
	setCondition(&conn.Status.Conditions, conn.Generation, dxv1.ConditionReady, ok, reason, msg)
	conn.Status.ObservedGeneration = conn.Generation
	conn.Status.LastCheckTime = now()
	if !ok {
		log.Info("DX connection not ready", "reason", reason, "message", msg)
	}
	if was := findCond(before.Conditions); was == nil || was.Status != findCond(conn.Status.Conditions).Status {
		etype := corev1.EventTypeNormal
		if !ok {
			etype = corev1.EventTypeWarning
		}
		r.Recorder.Eventf(&conn, nil, etype, reason, "Check", "%s", msg)
	}
	if err := r.Status().Patch(ctx, &conn, client.MergeFrom(orig)); err != nil {
		return ctrl.Result{}, err
	}
	if !ok {
		return ctrl.Result{RequeueAfter: retryFailed}, nil
	}
	return ctrl.Result{RequeueAfter: resync}, nil
}

func (r *DXConnectionReconciler) check(ctx context.Context, conn *dxv1.DXConnection) (bool, string, string) {
	cfg, _, err := ConnectionConfig(ctx, r.Client, conn)
	if err != nil {
		return false, "SecretError", err.Error()
	}
	if err := r.DXClient.Available(); err != nil {
		return false, "DXClientUnavailable", err.Error()
	}
	cl, err := dx.NewClient(cfg) // a fresh session: this is a real credentials check
	if err != nil {
		return false, "ConfigError", err.Error()
	}
	if err := cl.Login(ctx); err != nil {
		return false, "LoginFailed", err.Error()
	}
	return true, "Connected", "signed in to " + cfg.BaseURL + " as " + cfg.Username
}

// SetupWithManager sets up the controller. Changes in the Secrets it uses trigger a new check.
func (r *DXConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dxv1.DXConnection{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.forSecret)).
		Named("dxconnection").
		Complete(r)
}

func (r *DXConnectionReconciler) forSecret(ctx context.Context, o client.Object) []reconcile.Request {
	var list dxv1.DXConnectionList
	if err := r.List(ctx, &list, client.InNamespace(o.GetNamespace())); err != nil {
		return nil
	}
	var out []reconcile.Request
	for _, c := range list.Items {
		if c.Spec.CredentialsSecret.Name == o.GetName() || (c.Spec.CASecret != nil && c.Spec.CASecret.Name == o.GetName()) {
			out = append(out, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&c)})
		}
	}
	return out
}
