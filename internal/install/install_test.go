package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
)

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/file", http.StatusFound)
			return
		}
		if r.URL.Path != "/file" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadVerifies(t *testing.T) {
	body := bytes.Repeat([]byte("kubectl"), 1000)
	srv := serve(t, body)
	dir := t.TempDir()
	var last int64
	got, err := Download(context.Background(), srv.Client(), srv.URL+"/redirect", sum(body), int64(len(body)), dir, "kubectl", func(done, _ int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(got); !bytes.Equal(data, body) || last != int64(len(body)) {
		t.Errorf("downloaded %d bytes, progress %d", len(data), last)
	}

	_, err = Download(context.Background(), srv.Client(), srv.URL+"/file", sum([]byte("other")), 0, dir, "bad", nil)
	var ce *ChecksumError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChecksumError, got %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "bad*")); len(left) != 0 {
		t.Errorf("a failed download left files: %v", left)
	}
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/missing", sum(body), 0, dir, "x", nil); err == nil {
		t.Error("want an error for HTTP 404")
	}
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("#!/bin/sh\necho hi\n")

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for _, name := range []string{"LICENSE", "kubelogin"} {
		w, _ := zw.Create(name)
		_, _ = w.Write(payload)
	}
	_ = zw.Close()
	zipPath := filepath.Join(dir, "a.zip")
	_ = os.WriteFile(zipPath, zbuf.Bytes(), 0o600)

	var tbuf bytes.Buffer
	gz := gzip.NewWriter(&tbuf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "./bin/", Typeflag: tar.TypeDir, Mode: 0o755})
	_ = tw.WriteHeader(&tar.Header{Name: "./bin/tool", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(payload))})
	_, _ = tw.Write(payload)
	_ = tw.Close()
	_ = gz.Close()
	tgzPath := filepath.Join(dir, "a.tar.gz")
	_ = os.WriteFile(tgzPath, tbuf.Bytes(), 0o600)

	tests := []struct {
		name, src string
		a         catalog.Artifact
		wantErr   bool
	}{
		{"zip", zipPath, catalog.Artifact{Archive: catalog.ArchiveZip, Path: "kubelogin"}, false},
		{"zip missing entry", zipPath, catalog.Artifact{Archive: catalog.ArchiveZip, Path: "nope"}, true},
		{"tar.gz", tgzPath, catalog.Artifact{Archive: catalog.ArchiveTarGz, Path: "bin/tool"}, false},
		{"raw", zipPath, catalog.Artifact{Archive: catalog.ArchiveNone}, false},
		{"not a zip", tgzPath, catalog.Artifact{Archive: catalog.ArchiveZip, Path: "x"}, true},
	}
	for _, tt := range tests {
		dst := filepath.Join(dir, tt.name+".out")
		err := Extract(tt.src, tt.a, dst)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v", tt.name, err)
			continue
		}
		if err == nil && tt.a.Archive != catalog.ArchiveNone {
			if got, _ := os.ReadFile(dst); !bytes.Equal(got, payload) {
				t.Errorf("%s: content = %q", tt.name, got)
			}
		}
	}
}

func TestPlaceReplaces(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "new"), filepath.Join(dir, "bin", "tool")
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	_ = os.WriteFile(dst, []byte("old"), 0o755)
	_ = os.WriteFile(src, []byte("new"), 0o600)
	if err := Place(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "new" {
		t.Errorf("dst = %q", got)
	}
}

func TestCheckReachable(t *testing.T) {
	srv := serve(t, []byte("x"))
	if host, err := CheckReachable(context.Background(), srv.Client(), srv.URL+"/redirect"); err != nil {
		t.Errorf("reachable server: %s %v", host, err)
	}
	srv.Close()
	host, err := CheckReachable(context.Background(), http.DefaultClient, srv.URL+"/file")
	if err == nil || host != "127.0.0.1" {
		t.Errorf("closed server: host %q, err %v", host, err)
	}
}
