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
