// Package install downloads pinned files, checks their SHA-256, unpacks them
// and moves them into place in one step, so an interrupted run never leaves
// a half-written file in the bin folder.
package install

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/fsutil"
)

// maxBinarySize protects against archives that unpack to huge files.
const maxBinarySize = 500 << 20

// ChecksumError means a download does not match its pinned SHA-256.
type ChecksumError struct{ Want, Got string }

func (e *ChecksumError) Error() string {
	return fmt.Sprintf("sha256 is %s, want %s", e.Got, e.Want)
}

// NewHTTPClient honors the system proxy variables and never turns off TLS
// verification.
func NewHTTPClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
	}}
}

// Progress is called while downloading.
type Progress func(done, total int64)

// Download fetches rawURL into dir and checks its SHA-256 while writing.
// It returns the path of the verified file.
func Download(ctx context.Context, client *http.Client, rawURL, wantSHA string, size int64, dir, name string, progress Progress) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = size
	}

	part := filepath.Join(dir, name+".part")
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
			_ = os.Remove(part)
		}
	}()
	h := sha256.New()
	pr := &progressReader{r: resp.Body, total: total, fn: progress}
	if _, err := io.Copy(io.MultiWriter(f, h), pr); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return "", &ChecksumError{Want: wantSHA, Got: got}
	}
	verified := filepath.Join(dir, name+".verified")
	if err := fsutil.Rename(part, verified); err != nil {
		return "", err
	}
	ok = true
	return verified, nil
}

type progressReader struct {
	r           io.Reader
	done, total int64
	fn          Progress
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.fn != nil && n > 0 {
		p.fn(p.done, p.total)
	}
	return n, err
}

// Extract writes the tool binary from a verified download to dst (a
// temporary file in the same folder as the final place).
func Extract(download string, a catalog.Artifact, dst string) error {
	switch a.Archive {
	case catalog.ArchiveNone:
		return fsutil.CopyFile(download, dst, 0o755)
	case catalog.ArchiveZip:
		return extractZip(download, a.Path, dst)
	case catalog.ArchiveTarGz:
		return extractTarGz(download, a.Path, dst)
	}
	return fmt.Errorf("unknown archive type %q", a.Archive)
}

func extractZip(archive, inner, dst string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if path.Clean(f.Name) != inner {
			continue
		}
		if f.UncompressedSize64 > maxBinarySize {
			return fmt.Errorf("%s is too large", inner)
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() { _ = rc.Close() }()
		return writeLimited(rc, dst)
	}
	return fmt.Errorf("%s is not in the archive", inner)
}

func extractTarGz(archive, inner, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s is not in the archive", inner)
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeReg && path.Clean(strings.TrimPrefix(hdr.Name, "./")) == inner {
			return writeLimited(tr, dst)
		}
	}
}

func writeLimited(r io.Reader, dst string) error {
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxBinarySize+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxBinarySize {
		err = errors.New("the file in the archive is too large")
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return err
}

// Place moves src to dst in one step and makes it executable. On Windows a
// running dst (kube-onramp itself) can't be replaced, but it can be renamed:
// it is moved aside to dst.old first and removed on a later run.
func Place(src, dst string) error {
	if runtime.GOOS != "windows" {
		if err := os.Chmod(src, 0o755); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	err := fsutil.Rename(src, dst)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	old := dst + ".old"
	_ = os.Remove(old)
	if rerr := os.Rename(dst, old); rerr != nil {
		return err
	}
	return fsutil.Rename(src, dst)
}

// CleanLeftovers removes temporary files from earlier, interrupted runs.
func CleanLeftovers(tmpDir, binDir string) {
	_ = os.RemoveAll(tmpDir)
	if matches, err := filepath.Glob(filepath.Join(binDir, "*.old")); err == nil {
		for _, m := range matches {
			_ = os.Remove(m) // fails while that old copy still runs; fine
		}
	}
	if matches, err := filepath.Glob(filepath.Join(binDir, ".*.tmp")); err == nil {
		for _, m := range matches {
			_ = os.Remove(m)
		}
	}
}

// CheckReachable makes a tiny request to rawURL and follows redirects. Any
// HTTP answer from the last host counts as reachable. On failure it returns
// the host that could not be reached.
func CheckReachable(ctx context.Context, client *http.Client, rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return hostOf(rawURL), err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return hostOf(uerr.URL), err
		}
		return hostOf(rawURL), err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	_ = resp.Body.Close()
	return "", nil
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return raw
	}
	return u.Hostname()
}
