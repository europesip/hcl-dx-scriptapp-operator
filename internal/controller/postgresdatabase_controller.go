package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
)

// PostgresDatabaseReconciler creates a database and its owner for the backend of an application, and writes
// a Secret with the connection details.
type PostgresDatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	*Shared
}

// +kubebuilder:rbac:groups=dx.europesip.com,resources=postgresdatabases,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=postgresdatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dx.europesip.com,resources=postgresdatabases/finalizers,verbs=update

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

func (r *PostgresDatabaseReconciler) names(d *dxv1.PostgresDatabase) (db, owner, secret string, err error) {
	db = d.Spec.Database
	if db == "" {
		db = strings.ReplaceAll(d.Name, "-", "_")
	}
	owner = d.Spec.Owner
	if owner == "" {
		owner = db
	}
	secret = d.Spec.SecretName
	if secret == "" {
		secret = d.Name + "-db"
	}
	if !identRe.MatchString(db) || !identRe.MatchString(owner) {
		err = fmt.Errorf("database and owner must be lower-case PostgreSQL identifiers: %q, %q", db, owner)
	}
	return
}

func (r *PostgresDatabaseReconciler) superuser(ctx context.Context, d *dxv1.PostgresDatabase, dbname string) (*pgx.Conn, error) {
	ns := d.Spec.SuperuserSecretNamespace
	if ns == "" {
		ns = d.Namespace
	}
	var s corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: d.Spec.SuperuserSecret.Name}, &s); err != nil {
		return nil, notReady("superuser secret %s/%s: %v", ns, d.Spec.SuperuserSecret.Name, err)
	}
	port := d.Spec.Port
	if port == 0 {
		port = 5432
	}
	ssl := d.Spec.SSLMode
	if ssl == "" {
		ssl = "prefer"
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword(string(s.Data["username"]), string(s.Data["password"])),
		Host: fmt.Sprintf("%s:%d", d.Spec.Host, port), Path: "/" + dbname, RawQuery: "sslmode=" + ssl + "&connect_timeout=10"}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return pgx.Connect(cctx, u.String())
}

// Reconcile makes sure the role, its password, the database and the extensions exist.
func (r *PostgresDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var d dxv1.PostgresDatabase
	if err := r.Get(ctx, req.NamespacedName, &d); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	db, owner, secretName, nerr := r.names(&d)
	if !d.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&d, Finalizer) {
			return ctrl.Result{}, nil
		}
		if d.Spec.DeletionPolicy == dxv1.DeletionDelete && nerr == nil {
			conn, err := r.superuser(ctx, &d, "postgres")
			if err != nil {
				r.Recorder.Eventf(&d, nil, corev1.EventTypeWarning, "DropFailed", "Delete", "%v", err)
				return ctrl.Result{RequeueAfter: retryFailed}, nil
			}
			_, err1 := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{db}.Sanitize()+" WITH (FORCE)")
			_, err2 := conn.Exec(ctx, "DROP ROLE IF EXISTS "+pgx.Identifier{owner}.Sanitize())
			_ = conn.Close(ctx)
			if err1 != nil || err2 != nil {
				r.Recorder.Eventf(&d, nil, corev1.EventTypeWarning, "DropFailed", "Delete", "%v %v", err1, err2)
				return ctrl.Result{RequeueAfter: retryFailed}, nil
			}
			r.Recorder.Eventf(&d, nil, corev1.EventTypeNormal, "Dropped", "Delete", "database %s and role %s dropped", db, owner)
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
	if nerr != nil {
		return result(false, "InvalidSpec", nerr.Error(), 0)
	}

	// 1. The Secret for the backend: the password is generated once and kept
	password, err := r.secret(ctx, &d, secretName, db, owner)
	if err != nil {
		return result(false, "SecretError", err.Error(), retryFailed)
	}
	d.Status.SecretName = secretName

	// 2. Role, database and extensions
	conn, err := r.superuser(ctx, &d, "postgres")
	if err != nil {
		reason := "ConnectFailed"
		if isNotReady(err) {
			reason = "Waiting"
		}
		return result(false, reason, err.Error(), retryFailed)
	}
	defer conn.Close(ctx) //nolint:errcheck
	role := pgx.Identifier{owner}.Sanitize()
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", owner).Scan(&exists); err != nil {
		return result(false, "SQLError", err.Error(), retryFailed)
	}
	stmt := "ALTER ROLE "
	if !exists {
		stmt = "CREATE ROLE "
	}
	// The password cannot be a bind parameter in CREATE/ALTER ROLE: it is quoted as a literal
	if _, err := conn.Exec(ctx, stmt+role+" WITH LOGIN PASSWORD "+quoteLiteral(password)); err != nil {
		return result(false, "SQLError", "role "+owner+": "+err.Error(), retryFailed)
	}
	if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", db).Scan(&exists); err != nil {
		return result(false, "SQLError", err.Error(), retryFailed)
	}
	if !exists {
		if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{db}.Sanitize()+" OWNER "+role); err != nil {
			return result(false, "SQLError", "database "+db+": "+err.Error(), retryFailed)
		}
		r.Recorder.Eventf(&d, nil, corev1.EventTypeNormal, "Created", "Create", "database %s created (owner %s)", db, owner)
	}
	if len(d.Spec.Extensions) > 0 {
		dbc, err := r.superuser(ctx, &d, db)
		if err != nil {
			return result(false, "ConnectFailed", err.Error(), retryFailed)
		}
		defer dbc.Close(ctx) //nolint:errcheck
		for _, e := range d.Spec.Extensions {
			if _, err := dbc.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{e}.Sanitize()); err != nil {
				return result(false, "SQLError", "extension "+e+": "+err.Error(), retryFailed)
			}
		}
	}
	return result(true, "Ready", fmt.Sprintf("database %s (owner %s) ready; connection details in Secret %s", db, owner, secretName), 30*time.Minute)
}

func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func randomPassword() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// secret creates or updates the Secret for the backend, keeping its password.
func (r *PostgresDatabaseReconciler) secret(ctx context.Context, d *dxv1.PostgresDatabase, name, db, owner string) (string, error) {
	var s corev1.Secret
	err := r.Get(ctx, client.ObjectKey{Namespace: d.Namespace, Name: name}, &s)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", err
	}
	password := string(s.Data["password"])
	if password == "" {
		password = randomPassword()
	}
	port := d.Spec.Port
	if port == 0 {
		port = 5432
	}
	ssl := d.Spec.SSLMode
	if ssl == "" {
		ssl = "prefer"
	}
	host := fmt.Sprintf("%s:%d", d.Spec.Host, port)
	uri := (&url.URL{Scheme: "postgresql", User: url.UserPassword(owner, password), Host: host, Path: "/" + db, RawQuery: "sslmode=" + ssl}).String()
	data := map[string][]byte{"username": []byte(owner), "password": []byte(password), "host": []byte(d.Spec.Host),
		"port": []byte(fmt.Sprint(port)), "database": []byte(db), "uri": []byte(uri),
		"jdbcUrl": []byte(fmt.Sprintf("jdbc:postgresql://%s/%s?sslmode=%s", host, db, ssl))}
	if apierrors.IsNotFound(err) {
		s = corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.Namespace}, Type: corev1.SecretTypeOpaque, Data: data}
		if err := controllerutil.SetControllerReference(d, &s, r.Scheme); err != nil {
			return "", err
		}
		return password, r.Create(ctx, &s)
	}
	s.Data = data
	return password, r.Update(ctx, &s)
}

// SetupWithManager sets up the controller.
func (r *PostgresDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dxv1.PostgresDatabase{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}))).
		Owns(&corev1.Secret{}).
		Named("postgresdatabase").
		Complete(r)
}
