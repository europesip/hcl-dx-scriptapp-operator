package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dxv1 "github.com/europesip/hcl-dx-scriptapp-operator/api/v1alpha1"
	"github.com/europesip/hcl-dx-scriptapp-operator/internal/db"
)

// DXDatabaseReconciler creates a database and its owner (PostgreSQL, Oracle or SQL Server) for the backend of an
// application, and writes a Secret with the connection details.
type DXDatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	*Shared
}

// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxdatabases,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxdatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=dxdatabases/finalizers,verbs=update

func target(d *dxv1.DXDatabase) (db.Target, string, error) {
	t := db.Target{Engine: string(d.Spec.Engine), Host: d.Spec.Host, Port: d.Spec.Port, Database: d.Spec.Database,
		Owner: d.Spec.Owner, Extensions: d.Spec.Extensions, SSLMode: d.Spec.SSLMode}
	if t.Database == "" {
		t.Database = strings.ReplaceAll(d.Name, "-", "_")
	}
	if d.Spec.Oracle != nil {
		t.Service, t.Tablespace = d.Spec.Oracle.Service, d.Spec.Oracle.Tablespace
	}
	if d.Spec.SQLServer != nil {
		t.Encrypt, t.TrustCert = d.Spec.SQLServer.Encrypt, d.Spec.SQLServer.TrustServerCertificate
	}
	secret := d.Spec.SecretName
	if secret == "" {
		secret = d.Name + "-db"
	}
	err := t.Normalise()
	return t, secret, err
}

func (r *DXDatabaseReconciler) admin(ctx context.Context, d *dxv1.DXDatabase) (db.Admin, error) {
	ns := d.Spec.AdminSecretNamespace
	if ns == "" {
		ns = d.Namespace
	}
	var s corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: d.Spec.AdminSecret.Name}, &s); err != nil {
		return db.Admin{}, notReady("admin secret %s/%s: %v", ns, d.Spec.AdminSecret.Name, err)
	}
	return db.Admin{Username: string(s.Data["username"]), Password: string(s.Data["password"])}, nil
}

// Reconcile makes sure the owner, its password and the database exist.
func (r *DXDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var d dxv1.DXDatabase
	if err := r.Get(ctx, req.NamespacedName, &d); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	t, secretName, terr := target(&d)
	if !d.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&d, Finalizer) {
			return ctrl.Result{}, nil
		}
		if d.Spec.DeletionPolicy == dxv1.DeletionDelete && terr == nil {
			a, err := r.admin(ctx, &d)
			if err == nil {
				err = t.Drop(ctx, a)
			}
			if err != nil {
				r.Recorder.Eventf(&d, nil, corev1.EventTypeWarning, "DropFailed", "Delete", "%v", err)
				return ctrl.Result{RequeueAfter: retryFailed}, nil
			}
			r.Recorder.Eventf(&d, nil, corev1.EventTypeNormal, "Dropped", "Delete", "%s database %s and user %s dropped", t.Engine, t.Database, t.Owner)
		}
		controllerutil.RemoveFinalizer(&d, Finalizer)
		return ctrl.Result{}, r.Update(ctx, &d)
	}
	if controllerutil.AddFinalizer(&d, Finalizer) {
		if err := r.Update(ctx, &d); err != nil {
			return ctrl.Result{}, err
		}
	}
	orig := d.DeepCopy()
	gen := d.Generation
	result := func(ok bool, reason, msg string, after time.Duration) (ctrl.Result, error) {
		setCondition(&d.Status.Conditions, gen, dxv1.ConditionReady, ok, reason, msg)
		if !ok && reason != "Waiting" {
			r.Recorder.Eventf(&d, nil, corev1.EventTypeWarning, reason, "Reconcile", "%s", msg)
		}
		d.Status.ObservedGeneration, d.Status.LastCheckTime = gen, now()
		if err := r.Status().Patch(ctx, &d, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: after}, nil
	}
	if terr != nil {
		return result(false, "InvalidSpec", terr.Error(), 0)
	}

	// 1. The Secret for the backend: the password is generated once and kept
	password, err := r.secret(ctx, &d, secretName, t)
	if err != nil {
		return result(false, "SecretError", err.Error(), retryFailed)
	}
	d.Status.SecretName = secretName

	// 2. Owner and database
	a, err := r.admin(ctx, &d)
	if err != nil {
		return result(false, "Waiting", err.Error(), retryFailed)
	}
	created, err := t.Ensure(ctx, a, password)
	if err != nil {
		return result(false, "SQLError", err.Error(), retryFailed)
	}
	if created {
		r.Recorder.Eventf(&d, nil, corev1.EventTypeNormal, "Created", "Create", "%s database %s created (owner %s)", t.Engine, t.Database, t.Owner)
	}
	return result(true, "Ready", fmt.Sprintf("%s database %s (owner %s) ready; connection details in Secret %s", t.Engine, t.Database, t.Owner, secretName), 30*time.Minute)
}

// secret creates or updates the Secret for the backend, keeping its password.
func (r *DXDatabaseReconciler) secret(ctx context.Context, d *dxv1.DXDatabase, name string, t db.Target) (string, error) {
	var s corev1.Secret
	err := r.Get(ctx, client.ObjectKey{Namespace: d.Namespace, Name: name}, &s)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", err
	}
	password := string(s.Data["password"])
	if password == "" {
		password = db.Password()
	}
	data := map[string][]byte{}
	for k, v := range t.Details(password) {
		data[k] = []byte(v)
	}
	if apierrors.IsNotFound(err) {
		s = corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.Namespace}, Type: corev1.SecretTypeOpaque, Data: data}
		if err := controllerutil.SetControllerReference(d, &s, r.Scheme); err != nil {
			return "", err
		}
		return password, r.Create(ctx, &s)
	}
	s.Data = data
	// Adopt a Secret written for an older resource of the operator (for example a PostgresDatabase of 0.2), so that
	// deleting that resource does not garbage-collect it and the password is kept
	refs := s.OwnerReferences[:0]
	for _, o := range s.OwnerReferences {
		if !strings.HasPrefix(o.APIVersion, dxv1.GroupVersion.Group+"/") || o.UID == d.UID {
			refs = append(refs, o)
		}
	}
	s.OwnerReferences = refs
	if err := controllerutil.SetControllerReference(d, &s, r.Scheme); err != nil {
		return "", err
	}
	return password, r.Update(ctx, &s)
}

// SetupWithManager sets up the controller.
func (r *DXDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dxv1.DXDatabase{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}))).
		Owns(&corev1.Secret{}).
		Named("dxdatabase").
		Complete(r)
}
