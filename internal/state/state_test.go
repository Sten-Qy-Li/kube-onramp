package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveOnlyWhenChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Team = "best-team"
	s.Tools = map[string]InstalledTool{"kubectl": {Version: "1.36.6", File: "kubectl.exe", SHA256: "abc"}}
	if changed, err := s.Save(path); err != nil || !changed {
		t.Fatalf("first Save = %v, %v", changed, err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s2.Save(path); err != nil || changed {
		t.Fatalf("second Save = %v, %v; want no change", changed, err)
	}
	if fi, _ := os.Stat(path); !fi.ModTime().Equal(old) {
		t.Errorf("modification time changed on a no-op save")
	}
}

func TestLoadBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err == nil {
		t.Error("want an error for a broken file")
	}
	if s == nil || s.Team != "" {
		t.Errorf("want an empty state, got %+v", s)
	}
}
