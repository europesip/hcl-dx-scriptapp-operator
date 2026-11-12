package dx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// DX with OpenID Connect: the REST login answers 200 without a cookie and the form login sets WAS_n<hash>.
func TestLoginFallsBackToFormLogin(t *testing.T) {
	var form, rest int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dx/api/core/v1/auth/login":
			rest++
			_, _ = w.Write([]byte(`{"statusCode":200,"message":"Login successful"}`))
		case "/wps/j_security_check":
			form++
			if r.FormValue("j_username") != "wpsadmin" || r.FormValue("j_password") != "secret" {
				http.Redirect(w, r, "/wps/portal?error", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "WAS_n123456", Value: "token", Path: "/"})
			http.Redirect(w, r, "/wps/", http.StatusFound)
		case "/wps/":
			t.Error("the form login redirect must not be followed (with OIDC it would go to the identity provider)")
		case "/dx/api/core/v1/dxmyrest/webcontent/contents/x":
			if ck, err := r.Cookie("WAS_n123456"); err != nil || ck.Value != "token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":"x"}`))
		}
	}))
	defer srv.Close()
	c, _ := NewClient(Config{BaseURL: srv.URL, Username: "wpsadmin", Password: "secret"})
	resp, err := c.Do(context.Background(), http.MethodGet, "/dx/api/core/v1/dxmyrest/webcontent/contents/x", nil, "")
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("call with the form login session: %v %v", err, resp)
	}
	if rest != 1 || form != 1 {
		t.Fatalf("REST login %d, form login %d: want 1 and 1", rest, form)
	}
	bad, _ := NewClient(Config{BaseURL: srv.URL, Username: "wpsadmin", Password: "wrong"})
	if err := bad.Login(context.Background()); err == nil {
		t.Fatal("wrong credentials must fail")
	}
}

// Without OIDC the REST login sets LtpaToken2 and no form login happens.
func TestLoginWithLtpaToken2(t *testing.T) {
	var form int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dx/api/core/v1/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "LtpaToken2", Value: "t", Path: "/"})
		case "/wps/j_security_check":
			form++
		}
	}))
	defer srv.Close()
	c, _ := NewClient(Config{BaseURL: srv.URL, Username: "u", Password: "p"})
	if err := c.Login(context.Background()); err != nil || form != 0 {
		t.Fatalf("err %v, form logins %d", err, form)
	}
}

// DX_FORM_LOGIN=false: the behaviour of 0.1.1 (REST login only)
func TestFormLoginCanBeTurnedOff(t *testing.T) {
	t.Setenv("DX_FORM_LOGIN", "false")
	var form int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wps/j_security_check" {
			form++
		}
	}))
	defer srv.Close()
	c, _ := NewClient(Config{BaseURL: srv.URL, Username: "u", Password: "p"})
	if err := c.Login(context.Background()); err != nil || form != 0 {
		t.Fatalf("err %v, form logins %d: want none", err, form)
	}
}

// After a restart of DX the old WAS_n cookie is no longer valid: the second login must not keep it (it made the
// form login look unnecessary and every call answered 401 until the operator restarted).
func TestReloginDropsStaleCookie(t *testing.T) {
	gen := "1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dx/api/core/v1/auth/login":
			_, _ = w.Write([]byte(`{"statusCode":200}`))
		case "/wps/j_security_check":
			http.SetCookie(w, &http.Cookie{Name: "WAS_n123456", Value: "token" + gen, Path: "/"})
			w.WriteHeader(http.StatusFound)
		case "/dx/api/core/v1/dxmyrest/webcontent/contents/x":
			if ck, err := r.Cookie("WAS_n123456"); err != nil || ck.Value != "token"+gen {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":"x"}`))
		}
	}))
	defer srv.Close()
	c, _ := NewClient(Config{BaseURL: srv.URL, Username: "wpsadmin", Password: "secret"})
	get := func() int {
		resp, err := c.Do(context.Background(), http.MethodGet, "/dx/api/core/v1/dxmyrest/webcontent/contents/x", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return resp.Status
	}
	if s := get(); s != http.StatusOK {
		t.Fatalf("first call: %d", s)
	}
	gen = "2" // DX restarted: the session of generation 1 is gone
	if s := get(); s != http.StatusOK {
		t.Fatalf("after the restart of DX: %d, want 200 (a new form login)", s)
	}
}

// DX sometimes answers XMLAccess with an empty HTTP 200: it is retried.
func TestXMLAccessRetriesEmptyAnswer(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			return // empty 200
		}
		_, _ = w.Write([]byte(`<request><status element="all" result="ok"/></request>`))
	}))
	defer srv.Close()
	c, _ := NewClient(Config{BaseURL: srv.URL, Username: "u", Password: "p"})
	out, err := c.XMLAccess(context.Background(), "<request/>")
	if err != nil || calls != 2 || out == "" {
		t.Fatalf("calls %d, err %v", calls, err)
	}
}

// The letter after WAS_ depends on the DX (WAS_n on the lab DX, WAS_p on another CF239): any WAS_ cookie is the session,
// the other Liberty cookies (WASReqURL..., WASOidc...) are not.
func TestSessionCookieNames(t *testing.T) {
	for name, want := range map[string]bool{
		"LtpaToken2": true, "LtpaToken": true, "WAS_n123456": true, "WAS_p11626475": true,
		"WASReqURLOidcp926085583": false, "WASOidcStatep926085583": false, "WASOidcSession": false, "JSESSIONID": false,
	} {
		if got := sessionCookie(name); got != want {
			t.Errorf("sessionCookie(%q) = %v, want %v", name, got, want)
		}
	}
}
