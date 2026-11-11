// Package fsutil has small file helpers: atomic writes and hashing.
package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// WriteFileAtomic writes data to a temporary file next to path and renames it
// into place, so readers see either the old or the new content, never half.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // no-op after a successful rename

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmp, perm); err != nil {
			return err
		}
	}
	return Rename(tmp, path)
}

// Rename is os.Rename with a few retries on Windows, where antivirus scanners
// and editors can hold a file open for a moment.
func Rename(from, to string) error {
	var err error
	for i := 0; i < 5; i++ {
		if err = os.Rename(from, to); err == nil || runtime.GOOS != "windows" {
			return err
		}
		time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
	}
	return err
}

// FileSHA256 returns the lowercase hex SHA-256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CopyFile copies src to dst (via WriteFileAtomic semantics: temp + rename).
func CopyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmp, perm); err != nil {
			return err
		}
	}
	return Rename(tmp, dst)
}
