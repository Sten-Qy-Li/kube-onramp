package app

import (
	"context"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/plan"
	"github.com/Sten-Qy-Li/kube-onramp/internal/ui"
)

// check inspects, renews the login if needed and verifies access. It never
// installs or changes anything else. Teammates run it before presentations.
func (a *App) check(ctx context.Context) int {
	a.total = 5
	a.step(1, "Checking your computer", "check the computer")
	lock, code, ok := a.preflightLocal(!a.opts.SkipLogin)
	if !ok {
		return code
	}
	defer lock.Release()
	if p := a.preflightNetwork(ctx, nil, !a.opts.SkipLogin); p != nil {
		return a.fail(p)
	}

	a.step(2, "Looking at what is installed", "look at what is installed")
	in, p := a.inspectTools(ctx, catalog.RoleCore)
	if p != nil {
		return a.fail(p)
	}
	if code, ok := a.resolveTeam(true); !ok {
		return code
	}
	if p := a.inspectKubeconfig(in); p != nil {
		return a.fail(p)
	}
	actions := a.buildPlan(in)
	broken := false
	for _, act := range actions {
		switch act.Kind {
		case plan.SwitchContext:
			a.ui.Item(ui.Warn, "The course cluster is not your default. Run: kubectl config use-context "+a.prof.Kubeconfig.Contexts.Staging)
			a.note(ui.Warn, "Default context")
		case plan.FixKubeconfigMode:
			a.ui.Item(ui.Warn, "Other users can read your Kubernetes settings file")
			a.note(ui.Warn, "Kubernetes settings file is not private")
		case plan.InstallSelf:
			a.log.Printf("kube-onramp is not in its own folder")
		default:
			broken = true
			a.ui.Item(ui.Fail, "Needs fixing: "+act.Text)
			a.note(ui.Fail, act.Text)
		}
	}
	if !broken {
		a.ui.Item(ui.OK, "Tools, PATH and Kubernetes settings are set up")
		a.note(ui.OK, "Tools, PATH and Kubernetes settings")
	}

	a.step(3, "Checking your login", "log in to the cluster through the browser")
	switch {
	case a.opts.SkipLogin:
		a.ui.Say("Skipped (--skip-login).")
	case broken:
		a.ui.Item(ui.Fail, "The login can't be checked until the problems above are fixed.")
	default:
		if p := a.loginStep(ctx); p != nil {
			return a.fail(p)
		}
	}

	a.step(4, "Checking your access to the cluster", "check access to the team namespace")
	switch {
	case a.opts.SkipLogin:
		a.ui.Say("Skipped (--skip-login).")
	case broken:
		a.ui.Item(ui.Fail, "Access can't be checked until the problems above are fixed.")
	default:
		if p := a.accessStep(ctx); p != nil {
			return a.fail(p)
		}
	}

	a.step(5, "Final check", "check a new terminal")
	a.freshShellStep(ctx)
	a.printSummary()
	a.ui.Blank()
	if broken || a.hasFailures() {
		a.ui.Say("Run kube-onramp to fix this.")
		return ExitFail
	}
	a.ui.Say("Everything works. Good luck with your presentation!")
	return ExitOK
}
