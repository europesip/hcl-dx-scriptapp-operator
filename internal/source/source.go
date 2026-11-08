// Package source gets the files of a Script Application or of a DAM collection: from an OCI image (read
// directly from the registry, nothing runs in the cluster), from a ConfigMap or from an archive URL. It
// prepares a working copy with the substitutions applied, and computes the hashes the operator compares.
package source

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dxv1 "github.com/europesip/hcl-dx-scriptapp-operator/api/v1alpha1"
)

// MaxFileSize: WCM stores each file of a Script Application in one element of at most 5,000,000 bytes; a
// bigger file makes the push fail half way and leaves an incomplete item. Files are checked before any push.
const MaxFileSize = 4_500_000

// File of a source.
type File struct {
	Path   string // relative, with "/"
	Size   int64
	SHA256 string
}

// Tree is the files of a source in a local folder.
type Tree struct {
	Dir      string
	Revision string // what identifies this version of the source (image digest, ConfigMap hash, archive SHA256)
	Files    []File
}

// Fetcher gets sources and keeps a cache by revision in CacheDir.
type Fetcher struct {
	Client   client.Client
	CacheDir string
	mu       sync.Mutex
}

// Fetch gets the files of a source.
func (f *Fetcher) Fetch(ctx context.Context, namespace string, src dxv1.Source) (*Tree, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(f.CacheDir, 0o700); err != nil {
		return nil, err
	}
	switch {
	case src.Image != nil:
		return f.fromImage(ctx, namespace, src.Image)
	case src.ConfigMap != nil:
		return f.fromConfigMap(ctx, namespace, src.ConfigMap)
	case src.URL != nil:
		return f.fromURL(ctx, src.URL)
	}
	return nil, errors.New("the source has no image, configMap or url")
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}

// cached returns the tree of a revision already extracted.
func (f *Fetcher) cached(rev string) (*Tree, bool) {
	dir := filepath.Join(f.CacheDir, shortHash(rev))
	if _, err := os.Stat(filepath.Join(dir, ".complete")); err != nil {
		return nil, false
	}
	t, err := scan(dir, rev)
	return t, err == nil
}

func (f *Fetcher) newDir(rev string) (string, error) {
	dir := filepath.Join(f.CacheDir, shortHash(rev))
	_ = os.RemoveAll(dir)
	return dir, os.MkdirAll(dir, 0o700)
}

func finish(dir, rev string) (*Tree, error) {
	if err := os.WriteFile(filepath.Join(dir, ".complete"), []byte(rev), 0o600); err != nil {
		return nil, err
	}
	return scan(dir, rev)
}

// scan lists the files of a folder (without the operator's marker file).
func scan(dir, rev string) (*Tree, error) {
	t := &Tree{Dir: dir, Revision: rev}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == ".complete" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h := sha256.Sum256(data)
		t.Files = append(t.Files, File{Path: rel, Size: int64(len(data)), SHA256: hex.EncodeToString(h[:])})
		return nil
	})
	sort.Slice(t.Files, func(i, j int) bool { return t.Files[i].Path < t.Files[j].Path })
	return t, err
}

// safeJoin joins a relative path to dir, refusing paths that leave it.
func safeJoin(dir, rel string) (string, error) {
	clean := path.Clean("/" + filepath.ToSlash(rel))
	if clean == "/" {
		return "", errors.New("empty path")
	}
	return filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(clean, "/"))), nil
}

// ---------------------------------------------------------------------------------------------- image

// keychain resolves registry credentials from dockerconfigjson Secrets.
type keychain struct{ auths map[string]authn.AuthConfig }

func (k keychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
	for _, key := range []string{r.RegistryStr(), "https://" + r.RegistryStr(), "http://" + r.RegistryStr()} {
		if a, ok := k.auths[key]; ok {
			return authn.FromConfig(a), nil
		}
	}
	return authn.Anonymous, nil
}

func (f *Fetcher) keychain(ctx context.Context, namespace string, refs []dxv1.LocalObjectReference) (authn.Keychain, error) {
	k := keychain{auths: map[string]authn.AuthConfig{}}
	for _, r := range refs {
		var s corev1.Secret
		if err := f.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: r.Name}, &s); err != nil {
			return nil, fmt.Errorf("pull secret %s: %w", r.Name, err)
		}
		var cfg struct {
			Auths map[string]authn.AuthConfig `json:"auths"`
		}
		if err := json.Unmarshal(s.Data[corev1.DockerConfigJsonKey], &cfg); err != nil {
			return nil, fmt.Errorf("pull secret %s is not a dockerconfigjson: %w", r.Name, err)
		}
		for reg, a := range cfg.Auths {
			k.auths[reg] = a
		}
	}
	return k, nil
}

func (f *Fetcher) fromImage(ctx context.Context, namespace string, src *dxv1.ImageSource) (*Tree, error) {
	ref, err := name.ParseReference(src.Ref)
	if err != nil {
		return nil, fmt.Errorf("image reference %q: %w", src.Ref, err)
	}
	kc, err := f.keychain(ctx, namespace, src.PullSecrets)
	if err != nil {
		return nil, err
	}
	img, err := remote.Image(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(kc), remote.WithPlatform(v1.Platform{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		return nil, fmt.Errorf("image %s: %w", src.Ref, err)
	}
	digest, err := img.Digest()
	if err != nil {
		return nil, err
	}
	sub := strings.Trim(path.Clean("/"+src.Path), "/") // path inside the image, e.g. "app"
	rev := ref.Context().String() + "@" + digest.String() + ":" + sub
	if t, ok := f.cached(rev); ok {
		return t, nil
	}
	dir, err := f.newDir(rev)
	if err != nil {
		return nil, err
	}
	rc := mutate.Extract(img) // flattened filesystem, whiteouts applied
	defer rc.Close()          //nolint:errcheck
	n, err := untar(rc, dir, sub)
	if err != nil {
		return nil, fmt.Errorf("image %s: %w", src.Ref, err)
	}
	if n == 0 {
		return nil, fmt.Errorf("image %s has no files under %s", src.Ref, "/"+sub)
	}
	return finish(dir, rev)
}

// untar extracts the regular files under prefix (a path without leading slash, "" for all) into dir.
func untar(r io.Reader, dir, prefix string) (int, error) {
	tr := tar.NewReader(r)
	n := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		p := strings.TrimPrefix(path.Clean("/"+h.Name), "/")
		if prefix != "" {
			if !strings.HasPrefix(p, prefix+"/") {
				continue
			}
			p = strings.TrimPrefix(p, prefix+"/")
		}
		dst, err := safeJoin(dir, p)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return n, err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return n, err
		}
		if _, err := io.Copy(out, io.LimitReader(tr, 200<<20)); err != nil {
			_ = out.Close()
			return n, err
		}
		_ = out.Close()
		n++
	}
}

// ---------------------------------------------------------------------------------------------- ConfigMap

func (f *Fetcher) fromConfigMap(ctx context.Context, namespace string, src *dxv1.ConfigMapSource) (*Tree, error) {
	var cm corev1.ConfigMap
	if err := f.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: src.Name}, &cm); err != nil {
		return nil, fmt.Errorf("ConfigMap %s: %w", src.Name, err)
	}
	files := map[string][]byte{}
	for k, v := range cm.Data {
		files[k] = []byte(v)
	}
	for k, v := range cm.BinaryData {
		files[k] = v
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("ConfigMap %s is empty", src.Name)
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s\x00%d\x00", k, len(files[k]))
		h.Write(files[k])
	}
	rev := "configmap:" + src.Name + "@sha256:" + hex.EncodeToString(h.Sum(nil))
	if t, ok := f.cached(rev); ok {
		return t, nil
	}
	dir, err := f.newDir(rev)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		dst, err := safeJoin(dir, strings.ReplaceAll(k, "__", "/"))
		if err != nil {
			return nil, fmt.Errorf("ConfigMap key %q: %w", k, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, files[k], 0o600); err != nil {
			return nil, err
		}
	}
	return finish(dir, rev)
}

// ---------------------------------------------------------------------------------------------- URL

func (f *Fetcher) fromURL(ctx context.Context, src *dxv1.URLSource) (*Tree, error) {
	sub := strings.Trim(path.Clean("/"+src.Path), "/")
	if src.SHA256 != "" {
		if t, ok := f.cached("url@sha256:" + strings.ToLower(src.SHA256) + ":" + sub); ok {
			return t, nil
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", src.URL, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", src.URL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 500<<20))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if src.SHA256 != "" && !strings.EqualFold(src.SHA256, got) {
		return nil, fmt.Errorf("download %s: SHA256 is %s, expected %s", src.URL, got, src.SHA256)
	}
	rev := "url@sha256:" + got + ":" + sub
	if t, ok := f.cached(rev); ok {
		return t, nil
	}
	dir, err := f.newDir(rev)
	if err != nil {
		return nil, err
	}
	var n int
	lower := strings.ToLower(strings.SplitN(src.URL, "?", 2)[0])
	switch {
	case strings.HasSuffix(lower, ".zip"):
		n, err = unzip(data, dir, sub)
	default: // .tar.gz / .tgz
		gz, gerr := gzip.NewReader(bytes.NewReader(data))
		if gerr != nil {
			return nil, fmt.Errorf("%s is not a .tar.gz or .zip archive", src.URL)
		}
		n, err = untar(gz, dir, sub)
	}
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("archive %s has no files under %q", src.URL, sub)
	}
	return finish(dir, rev)
}

func unzip(data []byte, dir, prefix string) (int, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		p := strings.TrimPrefix(path.Clean("/"+zf.Name), "/")
		if prefix != "" {
			if !strings.HasPrefix(p, prefix+"/") {
				continue
			}
			p = strings.TrimPrefix(p, prefix+"/")
		}
		dst, err := safeJoin(dir, p)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return n, err
		}
		rc, err := zf.Open()
		if err != nil {
			return n, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 200<<20))
		_ = rc.Close()
		if err != nil {
			return n, err
		}
		if err := os.WriteFile(dst, b, 0o600); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
