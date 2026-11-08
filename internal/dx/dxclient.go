package dx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DXClient runs the HCL DXClient command line tool that ships in the operator image (/dxclient). DXClient
// keeps its connection settings in files of its own folder, so runs are serialised: only one at a time.
type DXClient struct {
	// Home of DXClient (default /dxclient, env DXCLIENT_HOME).
	Home string
	// Node binary (default "node" from PATH).
	Node string
	// LicenseAccepted must be true: the operator does not run DXClient without the explicit acceptance of its
	// licence (Helm value dxclient.acceptLicense, env DXCLIENT_ACCEPT_LICENSE=true).
	LicenseAccepted bool
	// PushAttempts and PushRetryDelay for the temporary errors of a DX that is still starting.
	PushAttempts   int
	PushRetryDelay time.Duration

	mu       sync.Mutex
	accepted bool
}

// NewDXClientFromEnv configures DXClient from the environment of the operator.
func NewDXClientFromEnv() *DXClient {
	home := os.Getenv("DXCLIENT_HOME")
	if home == "" {
		home = "/dxclient"
	}
	return &DXClient{Home: home, Node: "node", LicenseAccepted: os.Getenv("DXCLIENT_ACCEPT_LICENSE") == "true",
		PushAttempts: 5, PushRetryDelay: 20 * time.Second}
}

// Available says whether DXClient can run here, and why not.
func (d *DXClient) Available() error {
	if !d.LicenseAccepted {
		return errors.New("the DXClient licence has not been accepted (Helm value dxclient.acceptLicense=true)")
	}
	if _, err := os.Stat(filepath.Join(d.Home, "bin", "dxclient")); err != nil {
		return fmt.Errorf("DXClient not found in %s", d.Home)
	}
	return nil
}

// run executes one DXClient command with the connection settings written to its configuration file (not to
// the command line: DXClient warns that its command lines end up in logs). The file is removed afterwards.
func (d *DXClient) run(ctx context.Context, cfg Config, extra map[string]string, timeout time.Duration, args ...string) (string, error) {
	if err := d.Available(); err != nil {
		return "", err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.accepted {
		c := exec.CommandContext(ctx, d.Node, append([]string{"bin/dxclient"}, "accept-license")...)
		c.Dir = d.Home
		c.Env = append(os.Environ(), "DXCLIENT_TYPE=container")
		_ = c.Run()
		d.accepted = true
	}
	scheme, host, port, err := cfg.hostPort()
	if err != nil {
		return "", err
	}
	base, err := os.ReadFile(filepath.Join(d.Home, "dist", "configuration", "config.json"))
	if err != nil {
		return "", fmt.Errorf("DXClient configuration template: %w", err)
	}
	conf := map[string]any{}
	if err := json.Unmarshal(base, &conf); err != nil {
		return "", err
	}
	// Every key DXClient reads must be a string: an undefined one makes it fail
	for k, v := range map[string]string{"dxProtocol": scheme, "hostname": host, "dxPort": port, "dxUsername": cfg.Username,
		"dxPassword": cfg.Password, "contenthandlerPath": cfg.ContentHandlerPath, "virtualPortalContext": cfg.VirtualPortalContext,
		"projectContext": ""} {
		conf[k] = v
	}
	for k, v := range extra {
		conf[k] = v
	}
	store := filepath.Join(d.Home, "store")
	if err := os.MkdirAll(store, 0o700); err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(conf, "", "  ")
	confFile := filepath.Join(store, "config.json")
	if err := os.WriteFile(confFile, data, 0o600); err != nil {
		return "", err
	}
	defer os.Remove(confFile) //nolint:errcheck

	env := append(os.Environ(), "DXCLIENT_TYPE=container") // licence and settings in its store folder, as in its image
	if len(cfg.CAPEM) > 0 {
		f, err := os.CreateTemp("", "dx-ca-*.crt")
		if err != nil {
			return "", err
		}
		defer os.Remove(f.Name()) //nolint:errcheck
		_, _ = f.Write(cfg.CAPEM)
		_ = f.Close()
		env = append(env, "NODE_EXTRA_CA_CERTS="+f.Name())
	}
	if cfg.Insecure {
		env = append(env, "NODE_TLS_REJECT_UNAUTHORIZED=0")
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(cctx, d.Node, append([]string{"bin/dxclient"}, args...)...)
	c.Dir = d.Home
	c.Env = env
	out, err := c.CombinedOutput()
	s := redact(string(out), cfg.Password)
	if cctx.Err() == context.DeadlineExceeded {
		return s, fmt.Errorf("DXClient did not finish in %s", timeout)
	}
	return s, err
}

func redact(s, secret string) string {
	if secret != "" {
		s = strings.ReplaceAll(s, secret, "********")
	}
	return s
}

var (
	transient = regexp.MustCompile(`(?i)Service Unavailable|Bad Gateway|Gateway Timeout|ECONNRESET|ECONNREFUSED|socket hang up|ETIMEDOUT`)
	contentID = regexp.MustCompile(`"contentId":"([^"]+)"`)
)

// PushRequest is a Script Application to push.
type PushRequest struct {
	SiteArea, Name, Title, Dir, MainHTMLFile, ProjectContext string
}

// Push pushes a Script Application (create or update) and returns its WCM content ID. DXClient prints
// "Content push was successful" even when DX rejects the content: only its exit code and the "status" of DX's
// answer are trusted. Temporary errors (WCM still starting) are retried.
func (d *DXClient) Push(ctx context.Context, cfg Config, p PushRequest) (string, string, error) {
	extra := map[string]string{}
	if p.ProjectContext != "" {
		extra["projectContext"] = p.ProjectContext
	}
	var out string
	var err error
	for attempt := 1; ; attempt++ {
		out, err = d.run(ctx, cfg, extra, 10*time.Minute, "deploy-scriptapplication", "push",
			"-wcmSiteArea", p.SiteArea, "-wcmContentName", p.Name, "-wcmContentTitle", p.Title,
			"-contentRoot", p.Dir, "-mainHtmlFile", p.MainHTMLFile)
		if err != nil && attempt < d.PushAttempts && transient.MatchString(out) {
			select {
			case <-ctx.Done():
				return "", out, ctx.Err()
			case <-time.After(d.PushRetryDelay):
			}
			continue
		}
		break
	}
	if err != nil || !strings.Contains(out, `"status":"success"`) {
		return "", out, fmt.Errorf("push of %q failed: %s", p.Name, lastLines(out, 3))
	}
	m := contentID.FindStringSubmatch(out)
	if m == nil {
		return "", out, fmt.Errorf("DX did not return the content ID of %q", p.Name)
	}
	return m[1], out, nil
}

// Undeploy removes a Script Application from WCM by its content ID.
func (d *DXClient) Undeploy(ctx context.Context, cfg Config, id string) (string, error) {
	out, err := d.run(ctx, cfg, nil, 5*time.Minute, "undeploy-scriptapplication", "--wcmContentId", id, "-f")
	if err != nil {
		return out, fmt.Errorf("undeploy of %s failed: %s", id, lastLines(out, 3))
	}
	return out, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
