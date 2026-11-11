// Package login runs `kubectl auth whoami`, which uses the kubeconfig exec
// entry and so starts the browser login through kubelogin when needed.
package login

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Timeout is how long the browser login may take.
const Timeout = 5 * time.Minute

// Kind classifies a login result.
type Kind int

// Login result kinds.
const (
	OK       Kind = iota
	TimedOut      // E-LOGIN-TIMEOUT
	Rejected      // E-LOGIN-REJECTED (after one retry with a clean cache)
	Ports         // E-PRE-PORTS
	Network       // E-CLUSTER-UNREACHABLE
	Other         // E-LOGIN-FAILED
)

// kubelogin prints this when it can't open the browser (int128/kubelogin
// v1.36.4, pkg/usecases/authentication/authcode/browser.go).
var urlRe = regexp.MustCompile(`Please visit the following URL in your browser(?: manually)?: (\S+)`)

// FindURL returns the login URL in a kubelogin output line, if any.
func FindURL(line string) (string, bool) {
	m := urlRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// kubectl wraps every failure of the exec plugin in this text, also when the
// plugin only timed out. It says nothing about the network (D39).
const pluginFailed = "unable to connect to the server: getting credentials"

// Classify maps kubectl/kubelogin output to a result kind. The order matters:
// network errors also contain "timeout", so they are checked first.
func Classify(output string, timedOut bool) Kind {
	o := strings.ReplaceAll(strings.ToLower(output), pluginFailed, "getting credentials")
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(o, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("could not start a local server", "address already in use", "only one usage of each socket address"):
		return Ports
	case has("unable to connect to the server", "dial tcp", "no such host", "connection refused",
		"network is unreachable", "tls handshake timeout", "i/o timeout", "connection reset"):
		return Network
	case timedOut || has("context deadline exceeded", "authentication timeout"):
		return TimedOut
	case has("unauthorized", "you must be logged in to the server", "provide credentials", "invalid_grant",
		"token is expired", "status code 401", "401 unauthorized"):
		return Rejected
	}
	return Other
}

// Options for WhoAmI.
type Options struct {
	Kubectl string // kube-onramp's own kubectl
	Context string // kubeconfig context to use
	Timeout time.Duration
	OnURL   func(url string)                 // called when kubelogin prints a login URL
	Log     func(format string, args ...any) // gets every output line
}

// Result of a successful login check.
type Result struct {
	Username string
}

// WhoAmI checks the login. If the cached token is still valid, it returns at
// once without a browser.
func WhoAmI(ctx context.Context, o Options) (Result, Kind, error) {
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, o.Kubectl, "auth", "whoami", "--context", o.Context, "-o", "json")
	// The exec entry uses interactiveMode: IfAvailable.
	cmd.Stdin = os.Stdin
	cmd.WaitDelay = 5 * time.Second
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, Other, err
	}
	if err := cmd.Start(); err != nil {
		return Result{}, Other, err
	}
	var errText strings.Builder
	scan := bufio.NewScanner(stderr)
	for scan.Scan() {
		line := scan.Text()
		errText.WriteString(line + "\n")
		if o.Log != nil {
			o.Log("kubectl: %s", line)
		}
		if u, ok := FindURL(line); ok && o.OnURL != nil {
			o.OnURL(u)
		}
	}
	_, _ = io.Copy(io.Discard, stderr)
	runErr := cmd.Wait()
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	if runErr != nil || timedOut {
		if runErr == nil {
			runErr = ctx.Err()
		}
		return Result{}, Classify(errText.String(), timedOut), errors.New(strings.TrimSpace(errText.String() + " " + runErr.Error()))
	}
	name, err := ParseUsername(stdout.Bytes())
	if err != nil {
		return Result{}, Other, err
	}
	return Result{Username: name}, OK, nil
}

// ParseUsername reads the user name from `kubectl auth whoami -o json`.
func ParseUsername(data []byte) (string, error) {
	var review struct {
		Status struct {
			UserInfo struct {
				Username string `json:"username"`
			} `json:"userInfo"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &review); err != nil {
		return "", err
	}
	if review.Status.UserInfo.Username == "" {
		return "", errors.New("whoami returned no user name")
	}
	return review.Status.UserInfo.Username, nil
}

// ClearTokenCache removes kube-onramp's own kubelogin token cache, so the
// next login starts fresh. Other token caches are not touched.
func ClearTokenCache(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
