package dx

// Themes of DX Compose: the static files live in the WebDAV file store (dav:fs-type1/themes/<folder>/) and the theme
// is registered with XMLAccess. The operator copies a base theme (Portal 8.5 by default) from the same DX, adds its
// own files, uploads the folder over WebDAV and registers the theme as a copy of the base theme registration.
// Everything uses basic authentication (WebDAV and /wps/config), like DXClient: it keeps working when DX signs users
// in through OpenID Connect, as long as the OIDC authFilter leaves out "Authorization: Basic" and /wps/config.
//
// (DXClient deploy-theme is not used: its registration step fails on CF239 with "Cannot read properties of undefined".)

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// ThemeRegistration is what the operator registers.
type ThemeRegistration struct {
	UniqueName     string
	Folder         string            // WebDAV folder of the theme
	BaseUniqueName string            // registration copied from this theme
	BaseFolder     string            // its WebDAV folder (references to it are pointed at Folder)
	Titles         map[string]string // locale -> title (only these locales are kept)
	Description    string
	Hash           string // kept as the theme parameter dxop.hash
	Profile        string // optional: resourceaggregation.profile (for example profiles/profile_europesip.json)
	Layout         string // optional: default layout template, relative to the theme folder (layout-templates/1Column)
}

func (c *Client) davBase() string {
	ch := c.cfg.ContentHandlerPath
	if ch == "" {
		ch = "/wps/mycontenthandler"
	}
	return c.cfg.BaseURL + ch + "/dav/fs-type1/themes/"
}

func davEscape(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// dav sends one WebDAV request with basic authentication and no session cookie.
func (c *Client) dav(ctx context.Context, method, rel string, body []byte, hdr map[string]string) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.davBase()+davEscape(rel), rd)
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	hc := *c.http
	hc.Jar = nil
	resp, err := hc.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("WebDAV %s %s: %w", method, rel, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, nil
}

var davHref = regexp.MustCompile(`<(?:[A-Za-z]+:)?href>([^<]+)</(?:[A-Za-z]+:)?href>`)

// parseDAVList returns the entries under folder (relative to the themes root, folders end with "/").
func parseDAVList(xml []byte, folder string) []string {
	const marker = "/dav/fs-type1/themes/"
	var out []string
	for _, m := range davHref.FindAllSubmatch(xml, -1) {
		h, err := url.PathUnescape(string(m[1]))
		if err != nil {
			h = string(m[1])
		}
		i := strings.Index(h, marker)
		if i < 0 {
			continue
		}
		rel := h[i+len(marker):]
		if rel == folder || !strings.HasPrefix(rel, folder) {
			continue
		}
		out = append(out, rel)
	}
	return out
}

// ThemeFolderExists says whether the WebDAV folder of a theme exists.
func (c *Client) ThemeFolderExists(ctx context.Context, folder string) (bool, error) {
	st, _, err := c.dav(ctx, "PROPFIND", strings.TrimSuffix(folder, "/")+"/", nil, map[string]string{"Depth": "0"})
	if err != nil {
		return false, err
	}
	switch {
	case st == http.StatusNotFound:
		return false, nil
	case st == 207 || st < 300:
		return true, nil
	}
	return false, fmt.Errorf("WebDAV PROPFIND %s answered HTTP %d", folder, st)
}

// DownloadTheme copies a theme folder of the WebDAV store into dir. It returns the number of files.
func (c *Client) DownloadTheme(ctx context.Context, folder, dir string) (int, error) {
	root := strings.TrimSuffix(folder, "/") + "/"
	pending, files := []string{root}, []string{}
	for len(pending) > 0 {
		cur := pending[0]
		pending = pending[1:]
		st, body, err := c.dav(ctx, "PROPFIND", cur, nil, map[string]string{"Depth": "1"})
		if err != nil {
			return 0, err
		}
		if st != 207 {
			return 0, fmt.Errorf("WebDAV PROPFIND %s answered HTTP %d", cur, st)
		}
		for _, e := range parseDAVList(body, cur) {
			if strings.HasSuffix(e, "/") {
				pending = append(pending, e)
			} else {
				files = append(files, e)
			}
		}
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("the theme folder %s is empty or does not exist", folder)
	}
	var mu sync.Mutex
	var firstErr error
	work := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range work {
				st, data, err := c.dav(ctx, http.MethodGet, f, nil, nil)
				if err == nil && st != http.StatusOK {
					err = fmt.Errorf("WebDAV GET %s answered HTTP %d", f, st)
				}
				if err == nil {
					dst := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(f, root)))
					if err = os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
						err = os.WriteFile(dst, data, 0o644)
					}
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	for _, f := range files {
		work <- f
	}
	close(work)
	wg.Wait()
	return len(files), firstErr
}

// UploadTheme uploads every file under dir to the WebDAV folder (creating the folders). It does not delete files that
// are in DX and not in dir.
func (c *Client) UploadTheme(ctx context.Context, folder, dir string) (int, error) {
	root := strings.TrimSuffix(folder, "/")
	var dirs, files []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." {
				dirs = append(dirs, rel)
			}
		} else {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	sort.Strings(dirs)
	for _, d := range append([]string{""}, dirs...) {
		target := root + "/"
		if d != "" {
			target = root + "/" + d + "/"
		}
		st, _, err := c.dav(ctx, "MKCOL", target, nil, nil)
		if err != nil {
			return 0, err
		}
		if st == http.StatusForbidden { // DX answers 403 (not 405) when the folder already exists
			if ok, _ := c.ThemeFolderExists(ctx, target); ok {
				continue
			}
		}
		if st >= 300 && st != http.StatusMethodNotAllowed && st != http.StatusConflict { // 405: it already exists
			return 0, fmt.Errorf("WebDAV MKCOL %s answered HTTP %d", target, st)
		}
	}
	var mu sync.Mutex
	var firstErr error
	work := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range work {
				data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
				var st int
				if err == nil {
					st, _, err = c.dav(ctx, http.MethodPut, root+"/"+f, data, map[string]string{"Content-Type": "application/octet-stream"})
				}
				if err == nil && st >= 300 {
					err = fmt.Errorf("WebDAV PUT %s answered HTTP %d", f, st)
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	for _, f := range files {
		work <- f
	}
	close(work)
	wg.Wait()
	return len(files), firstErr
}

// DeleteThemeFolder deletes the WebDAV folder of a theme (missing is fine).
func (c *Client) DeleteThemeFolder(ctx context.Context, folder string) error {
	st, _, err := c.dav(ctx, http.MethodDelete, strings.TrimSuffix(folder, "/")+"/", nil, nil)
	if err != nil {
		return err
	}
	if st >= 300 && st != http.StatusNotFound {
		return fmt.Errorf("WebDAV DELETE %s answered HTTP %d", folder, st)
	}
	return nil
}

// ThemeExportXML exports a theme registration by unique name.
func ThemeExportXML(uniqueName string) string {
	return fmt.Sprintf(xmlHead, "export", "") + fmt.Sprintf("    <theme action=\"export\" uniquename=\"%s\"/>\n", esc(uniqueName)) + xmlTail
}

// ThemeDeleteXML deletes a theme registration by unique name.
func ThemeDeleteXML(uniqueName string) string {
	return fmt.Sprintf(xmlHead, "update", "") + fmt.Sprintf("    <theme action=\"delete\" uniquename=\"%s\"/>\n", esc(uniqueName)) + xmlTail
}

// ParseThemeExport reads an export result like ParsePageExport (EJPXA0240E = not found).
func ParseThemeExport(result, uniqueName string) (PageExport, error) {
	ok, msg := XMLAccessOK(result)
	if !ok {
		if strings.Contains(result, "EJPXA0240E") {
			return PageExport{}, nil
		}
		return PageExport{}, fmt.Errorf("XMLAccess export of theme %s failed: %s", uniqueName, msg)
	}
	if !regexp.MustCompile(`<theme [^>]*uniquename="` + regexp.QuoteMeta(uniqueName) + `"`).MatchString(result) {
		return PageExport{}, nil
	}
	e := PageExport{Exists: true}
	for _, m := range paramValue.FindAllStringSubmatch(result, -1) {
		if m[1] == HashParameter {
			e.Hash = strings.TrimSpace(m[2])
		}
	}
	return e, nil
}

var (
	reThemeOpen  = regexp.MustCompile(`<theme ([^>]*)>`)
	reLocale     = regexp.MustCompile(`(?s)<localedata locale="([^"]+)">.*?</localedata>\s*`)
	reDisallow   = regexp.MustCompile(`(?s)<parameter name="com\.ibm\.portal\.theme\.disallow".*?</parameter>\s*`)
	reIsTemplate = regexp.MustCompile(`(<parameter name="com\.ibm\.portal\.isTemplate"[^>]*><!\[CDATA\[)true(\]\]>)`)
	reStatus     = regexp.MustCompile(`\s*<status [^>]*/>`)
	reBuild      = regexp.MustCompile(` (build|version)="[^"]*"`)
	reHashParam  = regexp.MustCompile(`(?s)<parameter name="` + regexp.QuoteMeta(HashParameter) + `".*?</parameter>\s*`)
)

// ThemeRegisterXML turns the export of the base theme into the registration of the new theme: a new object ID and
// unique name, not the default theme, its own WebDAV folder, not a template, only the declared titles (DX fails on
// the import with every exported language), the dxop.hash parameter and without the export's status.
func ThemeRegisterXML(baseExport string, r ThemeRegistration) (string, error) {
	i := strings.Index(baseExport, "<request")
	if i < 0 || !reThemeOpen.MatchString(baseExport) {
		return "", fmt.Errorf("the export of the base theme %s has no theme", r.BaseUniqueName)
	}
	req := baseExport[i:]
	req = reThemeOpen.ReplaceAllStringFunc(req, func(m string) string {
		a := reThemeOpen.FindStringSubmatch(m)[1]
		a = regexp.MustCompile(` objectid="[^"]*"`).ReplaceAllString(a, ` objectid="dxopTheme"`)
		a = regexp.MustCompile(` uniquename="[^"]*"`).ReplaceAllString(a, ` uniquename="`+esc(r.UniqueName)+`"`)
		a = strings.Replace(a, ` default="true"`, ` default="false"`, 1)
		return "<theme " + a + ">"
	})
	baseRef := "dav:fs-type1/themes/" + strings.TrimSuffix(r.BaseFolder, "/") + "/"
	req = strings.ReplaceAll(req, baseRef, "dav:fs-type1/themes/"+strings.TrimSuffix(r.Folder, "/")+"/")
	req = reDisallow.ReplaceAllString(req, "")
	req = reIsTemplate.ReplaceAllString(req, "${1}false${2}")
	req = reHashParam.ReplaceAllString(req, "")
	locales := make([]string, 0, len(r.Titles))
	for l := range r.Titles {
		locales = append(locales, l)
	}
	sort.Strings(locales)
	var ld strings.Builder
	for _, l := range locales {
		fmt.Fprintf(&ld, "<localedata locale=\"%s\"><title>%s</title><description>%s</description></localedata>\n            ", esc(l), esc(r.Titles[l]), esc(r.Description))
	}
	first := true
	req = reLocale.ReplaceAllStringFunc(req, func(string) string {
		if first {
			first = false
			return ld.String()
		}
		return ""
	})
	if r.Profile != "" {
		req = setParam(req, "resourceaggregation.profile", r.Profile)
	}
	if r.Layout != "" {
		req = setParam(req, "com.ibm.portal.layout.template.href", "dav:fs-type1/themes/"+strings.TrimSuffix(r.Folder, "/")+"/"+strings.Trim(r.Layout, "/")+"/")
	}
	// dxop.hash: with the other parameters (the schema wants them after the localedata elements)
	if i := strings.Index(req, "<parameter "); i >= 0 {
		req = req[:i] + param(HashParameter, r.Hash) + "\n            " + req[i:]
	} else {
		req = strings.Replace(req, "</theme>", param(HashParameter, r.Hash)+"\n    </theme>", 1)
	}
	req = reStatus.ReplaceAllString(req, "")
	req = reBuild.ReplaceAllString(req, "")
	req = regexp.MustCompile(`<request ([^>]*)type="export"`).ReplaceAllString(req, `<request ${1}type="update" create-oids="true"`)
	req = strings.Replace(req, `type="update" create-oids="true" create-oids="true"`, `type="update" create-oids="true"`, 1)
	req = regexp.MustCompile(`<theme ([^>]*)action="export"`).ReplaceAllString(req, `<theme ${1}action="update"`)
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + req, nil
}

// setParam sets the value of a theme parameter of an export, adding the parameter if it is missing.
func setParam(req, name, value string) string {
	re := regexp.MustCompile(`(<parameter name="` + regexp.QuoteMeta(name) + `"[^>]*><!\[CDATA\[)[^\]]*(\]\]></parameter>)`)
	if re.MatchString(req) {
		return re.ReplaceAllString(req, "${1}"+strings.ReplaceAll(value, "$", "$$")+"${2}")
	}
	if i := strings.Index(req, "<parameter "); i >= 0 {
		return req[:i] + param(name, value) + "\n            " + req[i:]
	}
	return strings.Replace(req, "</theme>", param(name, value)+"\n    </theme>", 1)
}

// AddToHead inserts html before </head> in the theme templates (theme.html, theme_sidenav.html and their language
// versions) of a downloaded theme. It returns how many files changed.
func AddToHead(dir, html string) (int, error) {
	if strings.TrimSpace(html) == "" {
		return 0, nil
	}
	n := 0
	tpl := regexp.MustCompile(`^theme(_sidenav)?(_[A-Za-z_]+)?\.html$`)
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !tpl.MatchString(path.Base(filepath.ToSlash(p))) {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		s := string(data)
		if !strings.Contains(s, "</head>") || strings.Contains(s, "<!-- dxop:head -->") {
			return nil
		}
		s = strings.Replace(s, "</head>", "<!-- dxop:head -->\n"+html+"\n</head>", 1)
		n++
		return os.WriteFile(p, []byte(s), 0o644)
	})
	return n, err
}
