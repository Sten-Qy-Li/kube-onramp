// Command kube-onramp gets a laptop ready for work on a Kubernetes cluster.
//
// This is the M0 scaffold: only `version` works. The setup flow arrives in
// v0.1.0 (docs/SPEC.md, section 13).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/profile"
)

// Set by GoReleaser through -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		_, _ = fmt.Fprintf(stdout, "kube-onramp %s (commit %s, built %s)\n", version, commit, date)
		return 0
	}
	// Fail loudly if the embedded data is broken, even in the scaffold.
	if _, err := catalog.Load(); err != nil {
		_, _ = fmt.Fprintf(stderr, "E-INTERNAL: the built-in tool list is broken: %v\n", err)
		return 1
	}
	if _, err := profile.LoadEmbedded(profile.DefaultID); err != nil {
		_, _ = fmt.Fprintf(stderr, "E-INTERNAL: the built-in profile is broken: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stderr, "This build of kube-onramp can't set up your computer yet.")
	_, _ = fmt.Fprintln(stderr, "Only 'kube-onramp version' works in this build.")
	return 1
}
