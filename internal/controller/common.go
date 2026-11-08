package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/recorder"

	dxv1 "github.com/europesip/hcl-dx-scriptapp-operator/api/v1alpha1"
	"github.com/europesip/hcl-dx-scriptapp-operator/internal/dx"
	"github.com/europesip/hcl-dx-scriptapp-operator/internal/source"
)

// Finalizer of every resource that changes DX.
const Finalizer = "dx.europesip.com/finalizer"

// Requeue delays.
const (
	waitShort   = 20 * time.Second // waiting for dependencies or a connection
	retryFailed = time.Minute      // after an error talking to DX
)

// Shared is what every reconciler uses: the DXClient runner, the source fetcher and the REST sessions.
type Shared struct {
	DXClient *dx.DXClient
	Fetcher  *source.Fetcher
	WorkDir  string
	Recorder recorder.EventRecorder

	mu       sync.Mutex
	sessions map[string]session
}

type session struct {
	fingerprint string
	client      *dx.Client
}

// Connection is a ready DXConnection with its REST client.
type Connection struct {
	Object *dxv1.DXConnection
	Config dx.Config
	Client *dx.Client
}

// Resync of the connection (minimum 1 minute).
func (c *Connection) Resync() time.Duration {
	d := 10 * time.Minute
	if c.Object.Spec.ResyncInterval != nil {
		d = c.Object.Spec.ResyncInterval.Duration
	}
	if d < time.Minute {
		d = time.Minute
	}
	return d
}

// ConnectionConfig reads a DXConnection and its Secrets into a dx.Config.
func ConnectionConfig(ctx context.Context, c client.Client, conn *dxv1.DXConnection) (dx.Config, string, error) {
	var cred corev1.Secret
	if err := c.Get(ctx, client.ObjectKey{Namespace: conn.Namespace, Name: conn.Spec.CredentialsSecret.Name}, &cred); err != nil {
		return dx.Config{}, "", fmt.Errorf("credentials secret %s: %w", conn.Spec.CredentialsSecret.Name, err)
	}
	user, pass := string(cred.Data["username"]), string(cred.Data["password"])
	if user == "" || pass == "" {
		return dx.Config{}, "", fmt.Errorf("credentials secret %s needs the keys username and password", cred.Name)
	}
	cfg := dx.Config{BaseURL: strings.TrimRight(conn.Spec.URL, "/"), Username: user, Password: pass,
		Insecure: conn.Spec.InsecureSkipTLSVerify, ContentHandlerPath: conn.Spec.ContentHandlerPath,
		VirtualPortalContext: conn.Spec.VirtualPortalContext}
	fp := conn.Spec.URL + "|" + cred.ResourceVersion + "|" + fmt.Sprint(conn.Spec.InsecureSkipTLSVerify) + "|" + conn.Spec.ContentHandlerPath
	if conn.Spec.CASecret != nil {
		var ca corev1.Secret
		if err := c.Get(ctx, client.ObjectKey{Namespace: conn.Namespace, Name: conn.Spec.CASecret.Name}, &ca); err != nil {
			return dx.Config{}, "", fmt.Errorf("CA secret %s: %w", conn.Spec.CASecret.Name, err)
		}
		cfg.CAPEM = ca.Data["ca.crt"]
		if len(cfg.CAPEM) == 0 {
			return dx.Config{}, "", fmt.Errorf("CA secret %s has no key ca.crt", ca.Name)
		}
		fp += "|" + ca.ResourceVersion
	}
	return cfg, fp, nil
}

// session returns a REST client for a connection, reused while the connection and its secrets do not change.
func (s *Shared) session(conn *dxv1.DXConnection, cfg dx.Config, fp string) (*dx.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]session{}
	}
	key := conn.Namespace + "/" + conn.Name
	if cur, ok := s.sessions[key]; ok && cur.fingerprint == fp {
		return cur.client, nil
	}
	cl, err := dx.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	s.sessions[key] = session{fingerprint: fp, client: cl}
	return cl, nil
}

// errNotReady: a dependency or the connection is not ready yet (not an error: requeue later).
type errNotReady struct{ msg string }

func (e errNotReady) Error() string { return e.msg }

func notReady(format string, a ...any) error { return errNotReady{msg: fmt.Sprintf(format, a...)} }

// Connection returns the DXConnection of a resource if it is ready.
func (s *Shared) Connection(ctx context.Context, c client.Client, namespace, name string) (*Connection, error) {
	var conn dxv1.DXConnection
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &conn); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, notReady("DXConnection %s does not exist", name)
		}
		return nil, err
	}
	if !meta.IsStatusConditionTrue(conn.Status.Conditions, dxv1.ConditionReady) {
		msg := "not checked yet"
		if cnd := meta.FindStatusCondition(conn.Status.Conditions, dxv1.ConditionReady); cnd != nil {
			msg = cnd.Message
		}
		return nil, notReady("DXConnection %s is not ready: %s", name, msg)
	}
	cfg, fp, err := ConnectionConfig(ctx, c, &conn)
	if err != nil {
		return nil, notReady("DXConnection %s: %v", name, err)
	}
	cl, err := s.session(&conn, cfg, fp)
	if err != nil {
		return nil, err
	}
	return &Connection{Object: &conn, Config: cfg, Client: cl}, nil
}

// Dependencies checks that every dependency is ready. It returns a notReady error naming the first that is not.
func Dependencies(ctx context.Context, c client.Client, namespace string, deps []dxv1.Dependency) error {
	for _, d := range deps {
		ready, why, err := dependencyReady(ctx, c, namespace, d)
		if err != nil {
			return err
		}
		if !ready {
			return notReady("waiting for %s %s: %s", d.Kind, d.Name, why)
		}
	}
	return nil
}

func dependencyReady(ctx context.Context, c client.Client, ns string, d dxv1.Dependency) (bool, string, error) {
	key := client.ObjectKey{Namespace: ns, Name: d.Name}
	get := func(obj client.Object) (bool, error) {
		if err := c.Get(ctx, key, obj); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	switch d.Kind {
	case "Deployment":
		var o appsv1.Deployment
		if ok, err := get(&o); !ok || err != nil {
			return false, "does not exist", err
		}
		want := int32(1)
		if o.Spec.Replicas != nil {
			want = *o.Spec.Replicas
		}
		if o.Status.ObservedGeneration >= o.Generation && o.Status.UpdatedReplicas == want && o.Status.AvailableReplicas >= want {
			return true, "", nil
		}
		return false, fmt.Sprintf("%d/%d available", o.Status.AvailableReplicas, want), nil
	case "StatefulSet":
		var o appsv1.StatefulSet
		if ok, err := get(&o); !ok || err != nil {
			return false, "does not exist", err
		}
		want := int32(1)
		if o.Spec.Replicas != nil {
			want = *o.Spec.Replicas
		}
		if o.Status.ReadyReplicas >= want {
			return true, "", nil
		}
		return false, fmt.Sprintf("%d/%d ready", o.Status.ReadyReplicas, want), nil
	case "Job":
		var o batchv1.Job
		if ok, err := get(&o); !ok || err != nil {
			return false, "does not exist", err
		}
		if o.Status.Succeeded > 0 {
			return true, "", nil
		}
		return false, "not completed", nil
	default: // resources of the operator: their Ready condition
		gv := d.APIVersion
		if gv == "" || gv == "apps/v1" {
			gv = dxv1.GroupVersion.String()
		}
		u := &unstructured.Unstructured{}
		gvk, _ := schema.ParseGroupVersion(gv)
		u.SetGroupVersionKind(gvk.WithKind(d.Kind))
		if ok, err := get(u); !ok || err != nil {
			return false, "does not exist", err
		}
		conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
		for _, x := range conds {
			m, _ := x.(map[string]any)
			if m["type"] == dxv1.ConditionReady {
				if m["status"] == "True" {
					return true, "", nil
				}
				msg, _ := m["message"].(string)
				return false, msg, nil
			}
		}
		return false, "not reconciled yet", nil
	}
}

// setCondition sets a condition with the generation of the object.
func setCondition(conds *[]metav1.Condition, gen int64, typ string, ok bool, reason, msg string) {
	st := metav1.ConditionFalse
	if ok {
		st = metav1.ConditionTrue
	}
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	meta.SetStatusCondition(conds, metav1.Condition{Type: typ, Status: st, Reason: reason, Message: msg, ObservedGeneration: gen})
}

func hashOf(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))[:32]
}

func now() *metav1.Time { t := metav1.Now(); return &t }

func isNotReady(err error) bool {
	var e errNotReady
	return errors.As(err, &e)
}

// forceToken is the value of the reconcile annotation (a change forces a push/upload).
func forceToken(o metav1.Object) string { return o.GetAnnotations()[dxv1.AnnotationReconcile] }

// Annotation where the operator remembers the last reconcile token it honoured.
const appliedTokenAnnotation = "dx.europesip.com/reconciled"

// findCond returns the Ready condition (nil if there is none).
func findCond(conds []metav1.Condition) *metav1.Condition {
	return meta.FindStatusCondition(conds, dxv1.ConditionReady)
}

// refersTo is a reference of a resource to another object of its namespace (for the watches).
type refersTo struct{ kind, name string }

// mapReferrers builds the event handler function that enqueues the resources of a list that refer to the
// changed object (same namespace, same kind and name).
func mapReferrers[L client.ObjectList](c client.Client, newList func() L, refs func(L) map[client.ObjectKey][]refersTo, kind string) func(context.Context, client.Object) []reconcile.Request {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		list := newList()
		if err := c.List(ctx, list, client.InNamespace(o.GetNamespace())); err != nil {
			return nil
		}
		var out []reconcile.Request
		for key, rs := range refs(list) {
			for _, r := range rs {
				if r.kind == kind && r.name == o.GetName() {
					out = append(out, reconcile.Request{NamespacedName: key})
					break
				}
			}
		}
		return out
	}
}
