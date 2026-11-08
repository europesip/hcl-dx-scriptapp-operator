package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// textExt are the files where ${NAME} placeholders are replaced.
var textExt = map[string]bool{".html": true, ".htm": true, ".js": true, ".mjs": true, ".css": true, ".json": true, ".txt": true, ".svg": true, ".xml": true, ".map": false}

var placeholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Prepared is a working copy of an application, ready to push.
type Prepared struct {
	Dir   string
	Hash  string // hash of every file after the substitutions, and of the extra settings
	Files int
	Bytes int64
}

// Prepare copies the tree to a working folder, applies the substitutions to the text files (only the names
// given: other ${...} are left alone), checks the main HTML file and the size of every file, and computes the
// hash of the result plus settings (WCM name, title...).
func Prepare(t *Tree, workDir, mainHTML string, subs map[string]string, settings ...string) (*Prepared, error) {
	if mainHTML != "" {
		found := false
		for _, f := range t.Files {
			if f.Path == mainHTML {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("the main HTML file %q is not in the source (%d files)", mainHTML, len(t.Files))
		}
	}
	_ = os.RemoveAll(workDir)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, err
	}
	h := sha256.New()
	keys := make([]string, 0, len(subs))
	for k := range subs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, s := range settings {
		fmt.Fprintf(h, "setting\x00%s\x00", s)
	}
	p := &Prepared{Dir: workDir}
	var big []string
	for _, f := range t.Files {
		data, err := os.ReadFile(filepath.Join(t.Dir, filepath.FromSlash(f.Path)))
		if err != nil {
			return nil, err
		}
		if len(subs) > 0 && textExt[strings.ToLower(filepath.Ext(f.Path))] {
			data = placeholder.ReplaceAllFunc(data, func(m []byte) []byte {
				if v, ok := subs[string(m[2:len(m)-1])]; ok {
					return []byte(v)
				}
				return m
			})
		}
		if len(data) > MaxFileSize {
			big = append(big, fmt.Sprintf("%s (%d bytes)", f.Path, len(data)))
		}
		dst := filepath.Join(workDir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return nil, err
		}
		fmt.Fprintf(h, "file\x00%s\x00%d\x00", f.Path, len(data))
		h.Write(data)
		p.Files++
		p.Bytes += int64(len(data))
	}
	if len(big) > 0 {
		return nil, fmt.Errorf("files over %d bytes, which WCM cannot store (split the bundle or serve them from the DAM): %s",
			MaxFileSize, strings.Join(big, ", "))
	}
	p.Hash = "sha256:" + hex.EncodeToString(h.Sum(nil))
	return p, nil
}

// HumanSize formats a size in bytes.
func HumanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// AssetName is the DAM asset name of a file of a source: its path with "/" replaced by "-".
func AssetName(rel string) string { return strings.ReplaceAll(rel, "/", "-") }
