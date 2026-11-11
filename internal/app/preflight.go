package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/install"
	"github.com/Sten-Qy-Li/kube-onramp/internal/kubeconfig"
	"github.com/Sten-Qy-Li/kube-onramp/internal/platform"
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
	"github.com/Sten-Qy-Li/kube-onramp/internal/state"
	"github.com/Sten-Qy-Li/kube-onramp/internal/ui"
)

const minFreeDisk = 1 << 30 // 1 GB

// preflightLocal runs the read-only checks that need no network: OS, the
// run lock, disk space, login ports and KUBECONFIG. ok is false when the run
// must stop with code.
func (a *App) preflightLocal(needPorts bool) (lock *platform.Lock, code int, ok bool) {
	if p := platform.CheckSupported(a.info); p != nil {
		return nil, a.fail(p), false
	}
	a.ui.Item(ui.OK, a.info.Name)

	lock, err := platform.AcquireLock(a.layout.LockFile)
	if errors.Is(err, platform.ErrLocked) {
		a.ui.Blank()
		a.ui.Say("kube-onramp is already running in another window.",
			"Wait until it is finished, or close it. Then try again.")
		return nil, ExitLocked, false
	}
	if err != nil {
		return nil, a.fail(problem.Install("its own folder", err)), false
	}

	if free, err := platform.FreeDiskBytes(a.layout.Home); err != nil {
		a.log.Printf("free disk space: %v", err)
	} else if free < minFreeDisk {
		lock.Release()
		return nil, a.fail(problem.PreDisk(free, a.layout.Home)), false
	}

	if needPorts {
		free := false
		for _, addr := range platform.LoginPorts {
			if platform.PortFree(addr) {
				free = true
				break
			}
			a.log.Printf("login port %s is busy", addr)
		}
		if !free {
			lock.Release()
			return nil, a.fail(problem.PrePorts()), false
		}
	}

	if env := os.Getenv("KUBECONFIG"); env != "" {
		a.ui.Say("Your KUBECONFIG variable is set, so kube-onramp uses this Kubernetes settings file:",
			"  "+kubeconfig.TargetFile())
	}

	st, err := state.Load(a.layout.StateFile)
	if err != nil {
		a.log.Printf("state: %v", err)
	}
	a.st = st
	if a.st.Tools == nil {
		a.st.Tools = map[string]state.InstalledTool{}
	}
	return lock, 0, true
}

// toolsToDownload is a quick guess (no programs are run) of which tools
// will need a download, so preflight can test those hosts and OS minimums.
func (a *App) toolsToDownload(roles ...catalog.Role) []*catalog.Tool {
	var out []*catalog.Tool
	for _, role := range roles {
		for _, t := range a.cat.ByRole(role) {
			rec, ok := a.st.Tools[t.ID]
			_, err := os.Stat(a.ownPath(t.Binary))
			if err != nil || !ok || rec.Version != t.Version {
				out = append(out, t)
			}
		}
	}
	return out
}

// preflightNetwork checks the OS minimums and that every host the run needs
// answers. cluster adds the OIDC issuer and the API server.
func (a *App) preflightNetwork(ctx context.Context, tools []*catalog.Tool, cluster bool) *problem.Problem {
	type check struct {
		name string
		run  func(context.Context) (host string, err error)
	}
	var checks []check
	for _, t := range tools {
		art, ok := t.Artifact(runtime.GOOS, runtime.GOARCH)
		if !ok {
			return problem.PreOS(a.info.Name, "a system that "+t.Name+" supports")
		}
		if supported, err := art.SupportsOS(a.info.Version); err == nil && !supported {
			return problem.PreOS(a.info.Name, t.Name+" "+t.Version+" needs version "+art.MinOS+" or newer")
		}
		url := art.URL
		checks = append(checks, check{"download " + t.Name, func(ctx context.Context) (string, error) {
			return install.CheckReachable(ctx, a.http, url)
		}})
	}
	if cluster {
		checks = append(checks,
			check{"OIDC issuer", a.checkIssuer},
			check{"API server", a.checkServer})
	}

	errs := make([]error, len(checks))
	hosts := make([]string, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hosts[i], errs[i] = c.run(ctx)
		}()
	}
	wg.Wait()
	for i, c := range checks {
		if errs[i] != nil {
			a.log.Printf("network check %q failed: %v", c.name, errs[i])
			return problem.PreNet(hosts[i], errs[i])
		}
		a.log.Printf("network check %q: ok", c.name)
	}
	if len(checks) > 0 {
		a.ui.Item(ui.OK, "Network: every server kube-onramp needs can be reached")
	}
	return nil
}

func (a *App) checkIssuer(ctx context.Context) (string, error) {
	issuer := a.prof.OIDC.IssuerURL
	host := hostOf(issuer)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return host, err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return host, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return host, fmt.Errorf("OIDC discovery answered HTTP %d", resp.StatusCode)
	}
	return "", nil
}

// checkServer calls GET /version with the profile CA. Any HTTP answer means
// the server is reachable; a 200 also gives the server version.
func (a *App) checkServer(ctx context.Context) (string, error) {
	host := hostOf(a.prof.Cluster.Server)
	cert, err := a.prof.CACertificate()
	if err != nil {
		return host, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.prof.Cluster.Server, "/")+"/version", nil)
	if err != nil {
		return host, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return host, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		var v struct {
			GitVersion string `json:"gitVersion"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v) == nil {
			a.serverVersion = strings.TrimPrefix(v.GitVersion, "v")
			a.log.Printf("server version: %s", a.serverVersion)
		}
	}
	return "", nil
}

func hostOf(raw string) string {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, ":/"); i >= 0 {
		s = s[:i]
	}
	return s
}
