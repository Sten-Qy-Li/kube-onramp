// Package kubeconfig adds, checks and removes the profile's entries in the
// user's kubeconfig. It uses client-go's clientcmd, so files are read and
// written by the same rules as kubectl. Everything that is not a profile entry
// is left as it is.
package kubeconfig

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"time"
	"unicode/utf8"

	"k8s.io/client-go/tools/clientcmd"
	api "k8s.io/client-go/tools/clientcmd/api"

	"github.com/Sten-Qy-Li/kube-onramp/internal/fsutil"
	"github.com/Sten-Qy-Li/kube-onramp/internal/profile"
)

// Want is the set of entries kube-onramp owns in the kubeconfig.
type Want struct {
	ClusterName    string
	UserName       string
	StagingContext string
	ProdContext    string
	Cluster        *api.Cluster
	AuthInfo       *api.AuthInfo
	Staging        *api.Context
	Prod           *api.Context
}

// Desired builds the entries for a profile. pluginBinary is the absolute path
// of kube-onramp's own kubectl-oidc_login, so login works even if PATH is
// wrong. tokenCacheDir isolates the login from older token caches.
func Desired(p *profile.Profile, stagingNS, prodNS, pluginBinary, tokenCacheDir string) (*Want, error) {
	ca, err := base64.StdEncoding.DecodeString(p.Cluster.CertificateAuthorityData)
	if err != nil {
		return nil, fmt.Errorf("profile CA: %w", err)
	}
	args := []string{
		"get-token",
		"--oidc-issuer-url=" + p.OIDC.IssuerURL,
		"--oidc-client-id=" + p.OIDC.ClientID,
	}
	args = append(args, p.OIDC.ExtraArgs...)
	args = append(args, "--token-cache-dir="+tokenCacheDir)

	cfg := api.NewConfig()
	names := p.Kubeconfig
	cfg.Clusters[names.Cluster] = &api.Cluster{Server: p.Cluster.Server, CertificateAuthorityData: ca}
	cfg.AuthInfos[names.User] = &api.AuthInfo{Exec: &api.ExecConfig{
		APIVersion:         p.Exec.APIVersion,
		Command:            pluginBinary,
		Args:               args,
		InteractiveMode:    api.IfAvailableExecInteractiveMode,
		ProvideClusterInfo: false,
	}}
	cfg.Contexts[names.Contexts.Staging] = &api.Context{Cluster: names.Cluster, AuthInfo: names.User, Namespace: stagingNS}
	cfg.Contexts[names.Contexts.Prod] = &api.Context{Cluster: names.Cluster, AuthInfo: names.User, Namespace: prodNS}

	// Round-trip through the serializer so the entries look exactly like
	// entries loaded from a file, and can be compared with them.
	data, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, err
	}
	norm, err := clientcmd.Load(data)
	if err != nil {
		return nil, err
	}
	return &Want{
		ClusterName:    names.Cluster,
		UserName:       names.User,
		StagingContext: names.Contexts.Staging,
		ProdContext:    names.Contexts.Prod,
		Cluster:        norm.Clusters[names.Cluster],
		AuthInfo:       norm.AuthInfos[names.User],
		Staging:        norm.Contexts[names.Contexts.Staging],
		Prod:           norm.Contexts[names.Contexts.Prod],
	}, nil
}

// File is a kubeconfig as found on disk.
type File struct {
	Path   string
	Exists bool
	Raw    []byte
	Mode   fs.FileMode
	Config *api.Config // nil when Broken
	Broken bool
	// BrokenReason is a short technical reason for the log.
	BrokenReason string
}

// TargetFile is the file kube-onramp writes, by the same rules kubectl uses
// for new entries: the KUBECONFIG variable if set, else ~/.kube/config.
func TargetFile() string {
	return clientcmd.NewDefaultPathOptions().GetDefaultFilename()
}

// Read loads a kubeconfig. A missing file is not an error. A file that can't
// be parsed is returned with Broken set; only I/O errors are errors.
func Read(path string) (*File, error) {
	f := &File{Path: path}
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		f.Config = api.NewConfig()
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%s is a folder, not a file", path)
	}
	f.Exists = true
	f.Mode = fi.Mode().Perm()
	f.Raw, err = os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f.Config, err = Decode(f.Raw)
	if err != nil {
		f.Config = nil
		f.Broken = true
		f.BrokenReason = err.Error()
	}
	return f, nil
}

// Decode parses kubeconfig bytes. It rejects encodings that kubectl can't
// read either (UTF-16, binary data), instead of guessing.
func Decode(data []byte) (*api.Config, error) {
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		return nil, errors.New("the file is UTF-16 text; kubectl needs UTF-8")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return nil, errors.New("the file is not UTF-8 text")
	}
	cfg, err := clientcmd.Load(data)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// Status says which profile entries are present and correct.
type Status struct {
	ClusterOK, UserOK, StagingOK, ProdOK bool
	// AnyPresent is true if at least one entry with a profile name exists.
	AnyPresent bool
	// CurrentOK is true if the current context is one of the profile's.
	CurrentOK bool
	// ModeOK is false if others can read the file (macOS, Linux only).
	ModeOK bool
}

// EntriesOK is true when all four entries match.
func (s Status) EntriesOK() bool { return s.ClusterOK && s.UserOK && s.StagingOK && s.ProdOK }

// Check compares a file with the wanted entries.
func Check(f *File, w *Want) Status {
	st := Status{ModeOK: true}
	if f.Exists && runtime.GOOS != "windows" && f.Mode&0o077 != 0 {
		st.ModeOK = false
	}
	if f.Config == nil {
		return st
	}
	c := f.Config
	cl, okCl := c.Clusters[w.ClusterName]
	ai, okAI := c.AuthInfos[w.UserName]
	sc, okS := c.Contexts[w.StagingContext]
	pc, okP := c.Contexts[w.ProdContext]
	st.AnyPresent = okCl || okAI || okS || okP
	st.ClusterOK = okCl && equal(cl, w.Cluster)
	st.UserOK = okAI && equal(ai, w.AuthInfo)
	st.StagingOK = okS && equal(sc, w.Staging)
	st.ProdOK = okP && equal(pc, w.Prod)
	st.CurrentOK = c.CurrentContext == w.StagingContext || c.CurrentContext == w.ProdContext
	return st
}

// Apply adds or replaces the profile entries in cfg (nil means a new file).
// The current context is switched to staging unless it already is one of
// the profile's contexts (so a user who chose prod keeps prod). It returns
// the context that was current before, if it was switched away from.
func Apply(cfg *api.Config, w *Want) (*api.Config, string) {
	if cfg == nil {
		cfg = api.NewConfig()
	}
	ensureMaps(cfg)
	cfg.Clusters[w.ClusterName] = w.Cluster.DeepCopy()
	cfg.AuthInfos[w.UserName] = w.AuthInfo.DeepCopy()
	cfg.Contexts[w.StagingContext] = w.Staging.DeepCopy()
	cfg.Contexts[w.ProdContext] = w.Prod.DeepCopy()
	previous := ""
	if cfg.CurrentContext != w.StagingContext && cfg.CurrentContext != w.ProdContext {
		previous = cfg.CurrentContext
		cfg.CurrentContext = w.StagingContext
	}
	return cfg, previous
}

// Remove deletes the profile entries from cfg. If the current context is a
// profile context, previousContext is restored when it still exists.
// It reports whether anything changed and whether cfg is now empty.
func Remove(cfg *api.Config, w *Want, previousContext string) (changed, empty bool) {
	ensureMaps(cfg)
	if _, ok := cfg.Clusters[w.ClusterName]; ok {
		delete(cfg.Clusters, w.ClusterName)
		changed = true
	}
	if _, ok := cfg.AuthInfos[w.UserName]; ok {
		delete(cfg.AuthInfos, w.UserName)
		changed = true
	}
	for _, name := range []string{w.StagingContext, w.ProdContext} {
		if _, ok := cfg.Contexts[name]; ok {
			delete(cfg.Contexts, name)
			changed = true
		}
	}
	if cfg.CurrentContext == w.StagingContext || cfg.CurrentContext == w.ProdContext {
		cfg.CurrentContext = ""
		if _, ok := cfg.Contexts[previousContext]; ok {
			cfg.CurrentContext = previousContext
		}
		changed = true
	}
	empty = len(cfg.Clusters) == 0 && len(cfg.AuthInfos) == 0 && len(cfg.Contexts) == 0 && cfg.CurrentContext == ""
	return changed, empty
}

// Encode serializes a config the way kubectl does.
func Encode(cfg *api.Config) ([]byte, error) {
	return clientcmd.Write(*cfg)
}

// WriteFile writes data atomically with mode 600 (on macOS and Linux).
func WriteFile(path string, data []byte) error {
	return fsutil.WriteFileAtomic(path, data, 0o600)
}

// FixMode sets mode 600 without changing the content.
func FixMode(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return os.Chmod(path, 0o600)
}

// Backup copies the original bytes to dir/kubeconfig-<timestamp>.yaml.
func Backup(raw []byte, dir string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "kubeconfig-"+now.Format("20060102-150405")+".yaml")
	for i := 1; ; i++ {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("kubeconfig-%s-%d.yaml", now.Format("20060102-150405"), i))
	}
	return path, os.WriteFile(path, raw, 0o600)
}

func ensureMaps(cfg *api.Config) {
	if cfg.Clusters == nil {
		cfg.Clusters = map[string]*api.Cluster{}
	}
	if cfg.AuthInfos == nil {
		cfg.AuthInfos = map[string]*api.AuthInfo{}
	}
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]*api.Context{}
	}
}

// equal compares two entries, ignoring where they were loaded from.
func equal[T any](a, b T) bool {
	strip := func(v T) T {
		switch x := any(v).(type) {
		case *api.Cluster:
			c := x.DeepCopy()
			c.LocationOfOrigin = ""
			return any(c).(T)
		case *api.AuthInfo:
			c := x.DeepCopy()
			c.LocationOfOrigin = ""
			return any(c).(T)
		case *api.Context:
			c := x.DeepCopy()
			c.LocationOfOrigin = ""
			return any(c).(T)
		}
		return v
	}
	return reflect.DeepEqual(strip(a), strip(b))
}
