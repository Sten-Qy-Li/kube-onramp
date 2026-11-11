// Command kube-onramp gets a laptop ready for work on a Kubernetes cluster:
// it installs kubectl and kubelogin, sets up PATH and the kubeconfig, logs
// the user in and checks access. See docs/SPEC.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/Sten-Qy-Li/kube-onramp/internal/app"
)

// Set by GoReleaser through -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, app.Run)
	stop()
	os.Exit(code)
}

type runner func(ctx context.Context, opts app.Options, version string) int

var commands = map[string]bool{"setup": true, "check": true, "reset": true, "version": true, "help": true}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, runApp runner) int {
	var o app.Options
	fs := flag.NewFlagSet("kube-onramp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Team, "team", "", "")
	fs.StringVar(&o.StagingNamespace, "staging-namespace", "", "")
	fs.StringVar(&o.ProdNamespace, "prod-namespace", "", "")
	fs.StringVar(&o.Profile, "profile", "", "")
	fs.BoolVar(&o.Minimal, "minimal", false, "")
	fs.BoolVar(&o.Yes, "yes", false, "")
	fs.BoolVar(&o.DryRun, "dry-run", false, "")
	fs.BoolVar(&o.Verbose, "verbose", false, "")
	fs.BoolVar(&o.NoColor, "no-color", false, "")
	fs.BoolVar(&o.Relogin, "relogin", false, "")
	fs.BoolVar(&o.SkipLogin, "skip-login", false, "") // hidden: tests and CI

	// The command may come before or after the flags.
	command := ""
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				usage(stdout)
				return app.ExitOK
			}
			_, _ = fmt.Fprintf(stderr, "%v\n\n", err)
			usage(stderr)
			return app.ExitUsage
		}
		if fs.NArg() == 0 {
			break
		}
		arg := fs.Arg(0)
		if command != "" || !commands[arg] {
			_, _ = fmt.Fprintf(stderr, "Unknown command or extra word: %q\n\n", arg)
			usage(stderr)
			return app.ExitUsage
		}
		command = arg
		rest = fs.Args()[1:]
	}

	switch command {
	case "version":
		_, _ = fmt.Fprintf(stdout, "kube-onramp %s (commit %s, built %s)\n", version, commit, date)
		return app.ExitOK
	case "help":
		usage(stdout)
		return app.ExitOK
	case "":
		command = "setup"
	}
	o.Command = command
	return runApp(ctx, o, version)
}

func usage(w io.Writer) {
	_, _ = fmt.Fprint(w, `kube-onramp gets your computer ready for the course Kubernetes cluster.

Usage:
  kube-onramp [setup] [flags]   Install and set up everything, then log in and check access.
  kube-onramp check [flags]     Check that everything works. Use this before a presentation.
  kube-onramp reset [flags]     Remove everything that kube-onramp added.
  kube-onramp version           Show the version.

Flags:
  --team <name>                 Your team name, for example best-team. It is remembered.
  --staging-namespace <name>    Use this staging namespace instead of the usual one.
  --prod-namespace <name>       Use this prod namespace instead of the usual one.
  --minimal                     Install only kubectl and kubelogin, no extra tools.
  --yes                         Don't wait for Enter (for scripts).
  --dry-run                     Show the plan, then stop. Nothing is changed.
  --relogin                     Forget the saved login and log in again.
  --verbose                     Show more details.
  --no-color                    Don't use colors (NO_COLOR also works).
  --profile <id>                The cluster profile (default: ut-devops).
`)
}
