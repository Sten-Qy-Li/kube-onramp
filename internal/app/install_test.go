package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/platform"
	"github.com/Sten-Qy-Li/kube-onramp/internal/report"
	"github.com/Sten-Qy-Li/kube-onramp/internal/state"
	"github.com/Sten-Qy-Li/kube-onramp/internal/ui"
)

// A new copy that doesn't start must never reach the bin folder: not as a
// new file, and not over a copy that is already there (D50). This is what
// happened with oha on a Windows without the Visual C++ runtime (D49).
func TestInstallToolKeepsBrokenCopyOutOfBin(t *testing.T) {
	body := []byte("this is not a program\n") // right checksum, but it can't start
	sum := sha256.Sum256(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()

	tool := &catalog.Tool{
		ID: "demo", Name: "Demo", Version: "1.2.3", Role: catalog.RoleExtra, Binary: "demo",
		VersionCommand: []string{"--version"}, VersionRegex: `demo ([0-9.]+)`,
		Identity: catalog.Identity{Command: []string{"--help"}, MustMatch: "Demo"},
		Platforms: map[string]catalog.Artifact{runtime.GOOS + "/" + runtime.GOARCH: {
			URL: srv.URL + "/v1.2.3/demo", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body)),
			Archive: catalog.ArchiveNone, MinOS: "1.0",
		}},
	}

	for _, old := range []string{"", "the old copy"} {
		layout := platform.LayoutAt(t.TempDir())
		own := filepath.Join(layout.Bin, tool.FileName(runtime.GOOS))
		if old != "" {
			if err := os.MkdirAll(layout.Bin, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(own, []byte(old), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		log := report.NewMemoryLogger(report.NewRedactor("", ""))
		a := &App{
			layout: layout, http: srv.Client(), log: log,
			ui:       ui.NewPlain(io.Discard, strings.NewReader(""), log),
			st:       &state.State{Tools: map[string]state.InstalledTool{}},
			versions: map[string]string{},
		}

		p := a.installTool(context.Background(), tool)
		if p == nil || p.Code != "E-INSTALL" {
			t.Fatalf("old=%q: installTool = %v, want E-INSTALL", old, p)
		}
		got, err := os.ReadFile(own)
		switch {
		case old == "" && err == nil:
			t.Errorf("a broken new copy was put into the bin folder")
		case old != "" && string(got) != old:
			t.Errorf("the copy in the bin folder was replaced by a broken one (now %q, err %v)", got, err)
		}
		if _, ok := a.st.Tools[tool.ID]; ok {
			t.Errorf("old=%q: a broken copy was recorded in the state file", old)
		}
		if left, _ := os.ReadDir(layout.Tmp); len(left) != 0 {
			t.Errorf("old=%q: files left in the temporary folder: %v", old, left)
		}
	}
}
