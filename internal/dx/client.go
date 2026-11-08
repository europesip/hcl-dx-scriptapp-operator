// Package dx talks to HCL DX: its REST APIs (Ring API for authentication and WCM queries, DAM API) and
// DXClient, the HCL command line tool, for what has no REST API (pushing Script Applications, XMLAccess).
package dx

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned when DX answers 404 for an object.
var ErrNotFound = errors.New("not found in DX")

// Config is how to reach a DX server.
type Config struct {
	// BaseURL, for example https://dx.example.com (no trailing slash).
	BaseURL  string
	Username string
	Password string
	// CAPEM: certificate (or CA) to trust, for a self-signed certificate. Empty: the system CAs.
	CAPEM []byte
	// Insecure: do not verify the certificate.
	Insecure bool
	// ContentHandlerPath, default /wps/mycontenthandler.
	ContentHandlerPath string
	// VirtualPortalContext for pushes into a virtual portal.
	VirtualPortalContext string
	// NoFormLogin turns off the fallback to the portal form login (DX with OpenID Connect, since 0.1.2): with it, only
	// the REST login is used, as up to 0.1.1. Set from the environment variable DX_FORM_LOGIN=false (chart value
	// dx.oidcCompatibility=false).
	NoFormLogin bool
}

// Host and port of the base URL, for DXClient.
func (c Config) hostPort() (scheme, host, port string, err error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", "", "", err
	}
	scheme, host, port = u.Scheme, u.Hostname(), u.Port()
	if port == "" {
		port = map[string]string{"https": "443", "http": "80"}[scheme]
	}
	return scheme, host, port, nil
}

// Client is a REST session with DX (its SSO cookie: LtpaToken2, or the name Liberty generates when DX signs in
// through OpenID Connect). Safe for concurrent use.
type Client struct {
	cfg     Config
	http    *http.Client
	mu      sync.Mutex
	session time.Time
}

// NewClient prepares a client; it signs in on the first call.
func NewClient(cfg Config) (*Client, error) {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("DX_FORM_LOGIN"))); v == "false" || v == "never" || v == "0" {
		cfg.NoFormLogin = true
	}
	if cfg.ContentHandlerPath == "" {
		cfg.ContentHandlerPath = "/wps/mycontenthandler"
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.Insecure} //nolint:gosec // opt-in for labs
	if len(cfg.CAPEM) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(cfg.CAPEM) {
			return nil, errors.New("the CA secret does not contain a PEM certificate")
		}
		tlsCfg.RootCAs = pool
	}
	return &Client{cfg: cfg, http: &http.Client{
		Jar:       newResetJar(),
		Timeout:   90 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg, Proxy: http.ProxyFromEnvironment, MaxIdleConnsPerHost: 4},
	}}, nil
}

// Config returns the configuration of the client.
func (c *Client) Config() Config { return c.cfg }

// Login signs in (POST /dx/api/core/v1/auth/login). Sessions are reused for 20 minutes.
func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked(ctx, false)
}

func (c *Client) loginLocked(ctx context.Context, force bool) error {
	if !force && time.Since(c.session) < 20*time.Minute {
		return nil
	}
	// Start from an empty cookie jar: after a restart of DX the old SSO cookie is no longer valid, and keeping it
	// would make the form login below look unnecessary (hasSessionCookie) and every call answer 401 forever.
	if j, ok := c.http.Jar.(*resetJar); ok {
		j.Reset()
	}
	body, _ := json.Marshal(map[string]string{"username": c.cfg.Username, "password": c.cfg.Password})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/dx/api/core/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("DX is not reachable: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("DX rejected the credentials (HTTP %d)", resp.StatusCode)
	case resp.StatusCode >= 300:
		return fmt.Errorf("DX login answered HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	// The Ring API returns the LtpaToken2 cookie of the Web Engine. With OpenID Connect on (openidConnectClient-1.0),
	// Liberty gives the SSO cookie a generated name (WAS_n...) and the REST login answers 200 without a cookie: sign
	// in with the form login of the portal instead, which the Ring API accepts too (it forwards every cookie).
	if !c.hasSessionCookie() && !c.cfg.NoFormLogin {
		if err := c.formLogin(ctx); err != nil {
			return err
		}
	}
	c.session = time.Now()
	return nil
}

// resetJar is a cookie jar that can be emptied (a new login starts without the cookies of the old session).
type resetJar struct {
	mu  sync.RWMutex
	jar *cookiejar.Jar
}

func newResetJar() *resetJar {
	j, _ := cookiejar.New(nil)
	return &resetJar{jar: j}
}

func (r *resetJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.jar.SetCookies(u, cookies)
}

func (r *resetJar) Cookies(u *url.URL) []*http.Cookie {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.jar.Cookies(u)
}

// Reset drops every cookie.
func (r *resetJar) Reset() {
	j, _ := cookiejar.New(nil)
	r.mu.Lock()
	r.jar = j
	r.mu.Unlock()
}

// sessionCookie reports whether a cookie is the SSO cookie of DX: LtpaToken2 or a name generated by Liberty.
func sessionCookie(name string) bool {
	return name == "LtpaToken2" || name == "LtpaToken" || strings.HasPrefix(name, "WAS_n")
}

func (c *Client) hasSessionCookie() bool {
	u, err := url.Parse(c.cfg.BaseURL + "/")
	if err != nil {
		return false
	}
	for _, ck := range c.http.Jar.Cookies(u) {
		if sessionCookie(ck.Name) {
			return true
		}
	}
	return false
}

// formLogin posts the credentials to <context root>/j_security_check (the portal login form), without following
// the redirect. The context root is taken from the content handler path (/wps/mycontenthandler -> /wps).
func (c *Client) formLogin(ctx context.Context) error {
	root := strings.TrimSuffix(c.cfg.ContentHandlerPath, "/mycontenthandler")
	form := url.Values{"j_username": {c.cfg.Username}, "j_password": {c.cfg.Password}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+root+"/j_security_check", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	noRedirect := *c.http
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noRedirect.Do(req)
	if err != nil {
		return fmt.Errorf("DX is not reachable: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if !c.hasSessionCookie() {
		return fmt.Errorf("DX did not open a session for %s (REST login without a cookie, form login HTTP %d): check the credentials", c.cfg.Username, resp.StatusCode)
	}
	return nil
}

// Response of a REST call.
type Response struct {
	Status int
	Body   []byte
}

func (r *Response) json(v any) error {
	if err := json.Unmarshal(r.Body, v); err != nil {
		return fmt.Errorf("DX answered something that is not JSON (HTTP %d): %.200s", r.Status, r.Body)
	}
	return nil
}

// Do calls DX with the session; it signs in again once if the session expired.
func (c *Client) Do(ctx context.Context, method, path string, body []byte, contentType string) (*Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		c.mu.Lock()
		err := c.loginLocked(ctx, attempt > 0)
		c.mu.Unlock()
		if err != nil {
			return nil, err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, _ := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, rd)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		// An expired session shows up as 401, or as the login page (HTML) after a redirect
		if (resp.StatusCode == http.StatusUnauthorized ||
			(resp.StatusCode == http.StatusOK && bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")))) && attempt == 0 {
			continue
		}
		return &Response{Status: resp.StatusCode, Body: data}, nil
	}
	return nil, errors.New("DX session could not be renewed")
}

func (c *Client) doJSON(ctx context.Context, method, path string, in, out any) (*Response, error) {
	var body []byte
	ct := ""
	if in != nil {
		body, _ = json.Marshal(in)
		ct = "application/json"
	}
	r, err := c.Do(ctx, method, path, body, ct)
	if err != nil {
		return nil, err
	}
	if r.Status == http.StatusNotFound {
		return r, ErrNotFound
	}
	if r.Status >= 300 {
		return r, fmt.Errorf("%s %s answered HTTP %d: %.300s", method, path, r.Status, r.Body)
	}
	if out != nil {
		return r, r.json(out)
	}
	return r, nil
}

// XMLAccess sends an XMLAccess request to the XML configuration endpoint of DX (/wps/config, the one
// DXClient uses), with basic authentication, and returns DX's XML answer (also when it reports a failure).
func (c *Client) XMLAccess(ctx context.Context, request string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/wps/config", strings.NewReader(request))
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
	hc := *c.http
	hc.Jar = nil // basic authentication only: with the session cookie as well, DX answers 403
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("XMLAccess: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	data, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(data, []byte("<request")) {
		return "", fmt.Errorf("XMLAccess answered HTTP %d without an XML result: %.200s", resp.StatusCode, data)
	}
	return string(data), nil
}

// ---------------------------------------------------------------------------------------------- WCM

// Content is a WCM content item, as the Ring API returns it.
type Content struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Updated string `json:"updated"`
	Title   any    `json:"title"`
	Path    []struct {
		Name string `json:"name"`
	} `json:"path"`
}

const ring = "/dx/api/core/v1/dxmyrest/webcontent"

// GetContent reads a WCM content item by ID (ErrNotFound if it does not exist).
func (c *Client) GetContent(ctx context.Context, id string) (*Content, error) {
	var out Content
	if _, err := c.doJSON(ctx, http.MethodGet, ring+"/contents/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindContentByName finds a WCM content item by its exact name (nil if there is none).
func (c *Client) FindContentByName(ctx context.Context, name string) (*Content, error) {
	var out struct {
		Contents []Content `json:"contents"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, ring+"/search?type=Content&limit=50&name="+url.QueryEscape(name), nil, &out); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	for i := range out.Contents {
		if out.Contents[i].Name == name {
			return &out.Contents[i], nil
		}
	}
	return nil, nil
}

// DeleteContent deletes a WCM content item (a Script Application) with WCM REST v2. DXClient's
// undeploy-scriptapplication is not used: it refuses while a page that used the application is only marked for
// deletion (DX removes deleted pages later, with its cleanup task).
func (c *Client) DeleteContent(ctx context.Context, id string) error {
	r, err := c.Do(ctx, http.MethodDelete, c.cfg.ContentHandlerPath+"/wcmrest-v2/contents/"+url.PathEscape(id), nil, "")
	if err != nil {
		return err
	}
	if r.Status == http.StatusNotFound || r.Status < 300 {
		return nil
	}
	return fmt.Errorf("delete of content %s answered HTTP %d: %.300s", id, r.Status, r.Body)
}

// ---------------------------------------------------------------------------------------------- DAM

const dam = "/dx/api/dam/v1"

// Collection of the DAM.
type Collection struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	AccessReferenceID string `json:"accessreferenceid"`
}

// Asset of the DAM (media item).
type Asset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FindCollection finds a DAM collection by its exact name (nil if there is none).
func (c *Client) FindCollection(ctx context.Context, name string) (*Collection, error) {
	var out struct {
		Contents []Collection `json:"contents"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, dam+"/collections?limit=100&query="+url.QueryEscape(name), nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Contents {
		if out.Contents[i].Name == name {
			return &out.Contents[i], nil
		}
	}
	return nil, nil
}

// GetCollection reads a collection by ID (ErrNotFound if it does not exist).
func (c *Client) GetCollection(ctx context.Context, id string) (*Collection, error) {
	var out Collection
	if _, err := c.doJSON(ctx, http.MethodGet, dam+"/collections/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCollection creates a DAM collection.
func (c *Client) CreateCollection(ctx context.Context, name, description string) (*Collection, error) {
	var out Collection
	if _, err := c.doJSON(ctx, http.MethodPost, dam+"/collections", map[string]string{"name": name, "description": description}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteCollection deletes a collection with its assets.
func (c *Client) DeleteCollection(ctx context.Context, id string) error {
	// ignoreMediaReferences: the assets may be used by Script Applications or pages (they then show a broken link)
	_, err := c.doJSON(ctx, http.MethodDelete, dam+"/collections/"+url.PathEscape(id)+"?ignoreMediaReferences=true", nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// SetCollectionAccess gives the User role on the collection to anonymous users and/or to all authenticated
// users. Granting a role that is already there answers an error, which is ignored.
func (c *Client) SetCollectionAccess(ctx context.Context, col *Collection, anonymous, authenticated bool) {
	rid := url.QueryEscape(col.AccessReferenceID)
	if anonymous {
		_, _ = c.Do(ctx, http.MethodPost, "/dx/api/core/v1/access/principals?resourceId="+rid+"&roleName=User", nil, "")
	}
	if authenticated || anonymous {
		body, _ := json.Marshal(map[string]any{"permissionList": []map[string]string{{"distinguishedName": "all authenticated portal users"}}})
		_, _ = c.Do(ctx, http.MethodPost, "/dx/api/core/v1/access/permissions?resourceId="+rid+"&roleName=User&type=virtual", body, "application/json")
	}
}

// ListAssets lists the assets of a collection.
func (c *Client) ListAssets(ctx context.Context, collectionID string) ([]Asset, error) {
	var all []Asset
	for offset := 0; ; offset += 200 {
		var out struct {
			Contents []Asset `json:"contents"`
		}
		if _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/collections/%s/items?limit=200&offset=%d", dam, url.PathEscape(collectionID), offset), nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Contents...)
		if len(out.Contents) < 200 {
			return all, nil
		}
	}
}

// GetAsset reads an asset (ErrNotFound if it does not exist).
func (c *Client) GetAsset(ctx context.Context, collectionID, assetID string) (*Asset, error) {
	var out Asset
	if _, err := c.doJSON(ctx, http.MethodGet, dam+"/collections/"+url.PathEscape(collectionID)+"/items/"+url.PathEscape(assetID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func multipartFile(name string, data []byte, extra map[string]string) ([]byte, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	for k, v := range extra {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	return buf.Bytes(), w.FormDataContentType()
}

// UploadAsset uploads a new asset to a collection.
func (c *Client) UploadAsset(ctx context.Context, collectionID, name string, data []byte) (*Asset, error) {
	body, ct := multipartFile(name, data, map[string]string{"name": name})
	r, err := c.Do(ctx, http.MethodPost, dam+"/collections/"+url.PathEscape(collectionID)+"/items", body, ct)
	if err != nil {
		return nil, err
	}
	if r.Status >= 300 {
		return nil, fmt.Errorf("upload of %s answered HTTP %d: %.300s", name, r.Status, r.Body)
	}
	var out struct {
		MediaInfo *Asset `json:"mediaInfo"`
	}
	if err := r.json(&out); err != nil || out.MediaInfo == nil {
		return nil, fmt.Errorf("upload of %s: unexpected answer %.300s", name, r.Body)
	}
	return out.MediaInfo, nil
}

// ReplaceAsset uploads a new version of an asset (same ID and URL).
func (c *Client) ReplaceAsset(ctx context.Context, collectionID, assetID, name string, data []byte) error {
	body, ct := multipartFile(name, data, map[string]string{"title": name})
	r, err := c.Do(ctx, http.MethodPut, dam+"/collections/"+url.PathEscape(collectionID)+"/items/"+url.PathEscape(assetID), body, ct)
	if err != nil {
		return err
	}
	if r.Status >= 300 {
		return fmt.Errorf("new version of %s answered HTTP %d: %.300s", name, r.Status, r.Body)
	}
	return nil
}

// DeleteAsset deletes an asset.
func (c *Client) DeleteAsset(ctx context.Context, collectionID, assetID string) error {
	_, err := c.doJSON(ctx, http.MethodDelete, dam+"/collections/"+url.PathEscape(collectionID)+"/items/"+url.PathEscape(assetID), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// AssetURL returns the URL of the original rendition of an asset (relative to the DX host). The renditions
// are generated after an upload: it waits up to maxWait for it.
func (c *Client) AssetURL(ctx context.Context, collectionID, assetID string, maxWait time.Duration) (string, error) {
	end := time.Now().Add(maxWait)
	for {
		var out struct {
			Contents []struct {
				RenditionType string `json:"renditionType"`
				BinaryURL     string `json:"binaryUrl"`
			} `json:"contents"`
		}
		if _, err := c.doJSON(ctx, http.MethodGet, dam+"/collections/"+url.PathEscape(collectionID)+"/items/"+url.PathEscape(assetID)+"/renditions", nil, &out); err != nil {
			return "", err
		}
		for _, r := range out.Contents {
			if r.RenditionType == "Original" && r.BinaryURL != "" {
				return dam + r.BinaryURL, nil
			}
		}
		if time.Now().After(end) {
			return "", errors.New("the DAM has not generated the original rendition yet")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
