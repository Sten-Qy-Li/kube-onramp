// Package access runs `kubectl auth can-i` checks against a namespace.
package access

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Sten-Qy-Li/kube-onramp/internal/profile"
)

// Result of one check.
type Result struct {
	Check   profile.Check
	Allowed bool
	Err     error // the check could not run (network, login, ...)
}

// Text is the check as people read it: "get pods".
func (r Result) Text() string { return r.Check.Verb + " " + r.Check.Resource }

// CanI runs one check. "no" is not an error; Err is set only when kubectl
// could not answer.
func CanI(ctx context.Context, kubectl, kubeContext, namespace string, c profile.Check) Result {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, kubectl, "auth", "can-i", c.Verb, c.Resource, //nolint:gosec // own kubectl
		"--namespace", namespace, "--context", kubeContext)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	answer := strings.TrimSpace(stdout.String())
	switch {
	case answer == "yes" && err == nil:
		return Result{Check: c, Allowed: true}
	case strings.HasPrefix(answer, "no"):
		return Result{Check: c, Allowed: false}
	}
	if err == nil {
		err = errors.New("unexpected answer")
	}
	return Result{Check: c, Err: fmt.Errorf("%w: %s %s", err, answer, strings.TrimSpace(stderr.String()))}
}

// Run runs all checks.
func Run(ctx context.Context, kubectl, kubeContext, namespace string, checks []profile.Check) []Result {
	out := make([]Result, 0, len(checks))
	for _, c := range checks {
		out = append(out, CanI(ctx, kubectl, kubeContext, namespace, c))
	}
	return out
}
