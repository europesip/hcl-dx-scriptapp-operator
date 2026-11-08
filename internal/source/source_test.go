package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dxv1 "github.com/europesip/hcl-dx-scriptapp-operator/api/v1alpha1"
)

func fetchCM(t *testing.T, data map[string]string, bin map[string][]byte) *Tree {
	t.Helper()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "files", Namespace: "ns"}, Data: data, BinaryData: bin}
	f := &Fetcher{Client: fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(cm).Build(), CacheDir: t.TempDir()}
	tree, err := f.Fetch(context.Background(), "ns", dxv1.Source{ConfigMap: &dxv1.ConfigMapSource{Name: "files"}})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestConfigMapSource(t *testing.T) {
	tree := fetchCM(t, map[string]string{"index.html": "<b>${A}</b>", "assets__app.js": "x=${A}"}, map[string][]byte{"img.png": {1, 2, 3}})
	var paths []string
	for _, f := range tree.Files {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, ",") != "assets/app.js,img.png,index.html" {
		t.Errorf("files: %v", paths)
	}
	again := fetchCM(t, map[string]string{"index.html": "<b>${A}</b>", "assets__app.js": "x=${A}"}, map[string][]byte{"img.png": {1, 2, 3}})
	if again.Revision != tree.Revision {
		t.Error("the same content must give the same revision")
	}
}

func TestPrepare(t *testing.T) {
	tree := fetchCM(t, map[string]string{"index.html": "<b>${A}</b> ${UNKNOWN}", "app.js": "x='${A}'"}, map[string][]byte{"img.png": []byte("${A}")})
	p, err := Prepare(tree, filepath.Join(t.TempDir(), "w"), "index.html", map[string]string{"A": "hello"}, "name")
	if err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(p.Dir, "index.html"))
	img, _ := os.ReadFile(filepath.Join(p.Dir, "img.png"))
	if string(html) != "<b>hello</b> ${UNKNOWN}" || string(img) != "${A}" {
		t.Errorf("substitutions: %q %q", html, img)
	}
	p2, _ := Prepare(tree, filepath.Join(t.TempDir(), "w"), "index.html", map[string]string{"A": "other"}, "name")
	p3, _ := Prepare(tree, filepath.Join(t.TempDir(), "w"), "index.html", map[string]string{"A": "hello"}, "other name")
	if p.Hash == p2.Hash || p.Hash == p3.Hash {
		t.Error("the hash must change with the substitutions and the settings")
	}
	if _, err := Prepare(tree, t.TempDir(), "main.html", nil); err == nil {
		t.Error("a missing main HTML file must be an error")
	}
}

func TestPrepareTooBig(t *testing.T) {
	tree := fetchCM(t, map[string]string{"index.html": "x"}, map[string][]byte{"big.js": make([]byte, MaxFileSize+1)})
	if _, err := Prepare(tree, t.TempDir(), "index.html", nil); err == nil || !strings.Contains(err.Error(), "big.js") {
		t.Errorf("files over the WCM limit must be refused: %v", err)
	}
}

func TestSafeJoin(t *testing.T) {
	d := t.TempDir()
	p, err := safeJoin(d, "../../etc/passwd")
	if err != nil || !strings.HasPrefix(p, d) {
		t.Errorf("paths must stay inside the folder: %s %v", p, err)
	}
}
