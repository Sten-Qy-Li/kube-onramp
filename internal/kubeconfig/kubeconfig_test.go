package kubeconfig

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unicode/utf16"

	api "k8s.io/client-go/tools/clientcmd/api"

	"github.com/Sten-Qy-Li/kube-onramp/internal/profile"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func testWant(t *testing.T) *Want {
	t.Helper()
	p, err := profile.LoadEmbedded(profile.DefaultID)
	if err != nil {
		t.Fatal(err)
	}
	w, err := Desired(p, "ns-best-team-staging", "ns-best-team-prod",
		"/home/test/.kube-onramp/bin/kubectl-oidc_login", "/home/test/.kube-onramp/cache/oidc-login")
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func utf16File(t *testing.T, src string) []byte {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xFE})
	for _, u := range utf16.Encode([]rune(string(data))) {
		buf.Write([]byte{byte(u), byte(u >> 8)})
	}
	return buf.Bytes()
}

func TestMergeGolden(t *testing.T) {
	w := testWant(t)
	tests := []struct {
		name   string
		input  []byte // nil: no file
		golden string
		broken bool
		prev   string
	}{
		{name: "no file", golden: "empty.golden"},
		{name: "empty file", input: []byte{}, golden: "empty.golden"},
		{name: "course file", input: mustRead(t, "testdata/course.yaml"), golden: "course.golden", prev: "etais-user@kubernetes"},
		{name: "other clusters", input: mustRead(t, "testdata/other.yaml"), golden: "other.golden", prev: "docker-desktop"},
		{name: "utf-16 file", input: utf16File(t, "testdata/course.yaml"), golden: "empty.golden", broken: true},
		{name: "invalid yaml", input: []byte("clusters: [\n  - {"), golden: "empty.golden", broken: true},
		{name: "binary garbage", input: []byte{0x00, 0x01, 0xC3, 0x28, 0xFF}, golden: "empty.golden", broken: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			if tt.input != nil {
				if err := os.WriteFile(path, tt.input, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			f, err := Read(path)
			if err != nil {
				t.Fatal(err)
			}
			if f.Broken != tt.broken {
				t.Fatalf("Broken = %v (%s), want %v", f.Broken, f.BrokenReason, tt.broken)
			}
			if st := Check(f, w); st.EntriesOK() || st.CurrentOK {
				t.Errorf("Check before Apply = %+v, want not OK", st)
			}

			cfg, prev := Apply(f.Config, w)
			if prev != tt.prev {
				t.Errorf("previous context = %q, want %q", prev, tt.prev)
			}
			out, err := Encode(cfg)
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", tt.golden)
			if *update {
				if err := os.WriteFile(golden, out, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if want := mustRead(t, golden); !bytes.Equal(out, want) {
				t.Errorf("merged file differs from %s:\n%s", golden, out)
			}

			// After writing, everything is OK and a second Apply changes nothing.
			if err := WriteFile(path, out); err != nil {
				t.Fatal(err)
			}
			f2, err := Read(path)
			if err != nil {
				t.Fatal(err)
			}
			if st := Check(f2, w); !st.EntriesOK() || !st.CurrentOK || !st.ModeOK {
				t.Errorf("Check after Apply = %+v, want all OK", st)
			}
			cfg2, _ := Apply(f2.Config, w)
			if again, _ := Encode(cfg2); !bytes.Equal(again, out) {
				t.Errorf("second Apply changed the file")
			}

			// Remove restores the original content (for files that could be read).
			changed, empty := Remove(cfg2, w, prev)
			if !changed {
				t.Error("Remove changed nothing")
			}
			if tt.broken {
				return
			}
			orig, _ := Decode(tt.input)
			if orig == nil {
				orig = api.NewConfig()
			}
			wantBytes, _ := Encode(orig)
			gotBytes, _ := Encode(cfg2)
			if !bytes.Equal(gotBytes, wantBytes) {
				t.Errorf("after Remove:\n%s\nwant:\n%s", gotBytes, wantBytes)
			}
			if wantEmpty := len(orig.Clusters) == 0; empty != wantEmpty {
				t.Errorf("empty = %v, want %v", empty, wantEmpty)
			}
		})
	}
}

// TestCanonicalFixture keeps testdata/other.canonical.yaml in the exact form
// clientcmd writes. The CI smoke tests start from it and expect the same
// bytes back after setup + reset.
func TestCanonicalFixture(t *testing.T) {
	cfg, err := Decode(mustRead(t, "testdata/other.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Encode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := "testdata/other.canonical.yaml"
	if *update {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(out, mustRead(t, path)) {
		t.Errorf("%s is not canonical; run go test ./internal/kubeconfig -update", path)
	}
}

func TestApplyKeepsProdContext(t *testing.T) {
	w := testWant(t)
	cfg, _ := Apply(nil, w)
	cfg.CurrentContext = w.ProdContext
	cfg, prev := Apply(cfg, w)
	if cfg.CurrentContext != w.ProdContext || prev != "" {
		t.Errorf("current = %q, prev = %q; want prod kept", cfg.CurrentContext, prev)
	}
}

func TestCheckFindsDifferences(t *testing.T) {
	w := testWant(t)
	cfg, _ := Apply(nil, w)
	cfg.Clusters[w.ClusterName].Server = "https://old.example.com:6443"
	cfg.AuthInfos[w.UserName].Exec.Command = "kubectl"
	data, _ := Encode(cfg)
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, _ := Read(path)
	st := Check(f, w)
	if st.ClusterOK || st.UserOK || !st.StagingOK || !st.ProdOK || !st.AnyPresent {
		t.Errorf("Check = %+v", st)
	}
	if runtime.GOOS != "windows" && st.ModeOK {
		t.Error("mode 644 should not be OK")
	}
}

func TestDecode(t *testing.T) {
	if _, err := Decode(append([]byte{0xEF, 0xBB, 0xBF}, mustRead(t, "testdata/other.yaml")...)); err != nil {
		t.Errorf("UTF-8 BOM: %v", err)
	}
	for name, data := range map[string][]byte{
		"utf-16 BE": {0xFE, 0xFF, 0x00, 'a'},
		"nul byte":  []byte("apiVersion: v1\x00"),
		"not yaml":  []byte("{{{"),
	} {
		if _, err := Decode(data); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestBackup(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 11, 8, 30, 0, 0, time.UTC)
	p1, err := Backup([]byte("one"), dir, now)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := Backup([]byte("two"), dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatal("second backup overwrote the first")
	}
	if got, _ := os.ReadFile(p1); string(got) != "one" {
		t.Errorf("first backup = %q", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
