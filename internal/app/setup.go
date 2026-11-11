package app

import (
	"context"
	"fmt"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/install"
	"github.com/Sten-Qy-Li/kube-onramp/internal/plan"
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
	"github.com/Sten-Qy-Li/kube-onramp/internal/state"
	"github.com/Sten-Qy-Li/kube-onramp/internal/ui"
)

// setup is the full reconcile: inspect, plan, act on the difference, verify.
func (a *App) setup(ctx context.Context) int {
	a.total = 8
	if !a.opts.Yes && !a.opts.DryRun && !a.ui.Interactive() {
		a.ui.Say("kube-onramp needs you to press Enter once, but it can't read from this window.",
			"Run it in a normal terminal window, or add --yes to start without asking.")
		return ExitUsage
	}

	// Step 1: read-only checks; any failure stops before anything changes.
	a.step(1, "Checking your computer", "check the computer before changing anything")
	lock, code, ok := a.preflightLocal(!a.opts.SkipLogin)
	if !ok {
		return code
	}
	defer lock.Release()
	install.CleanLeftovers(a.layout.Tmp, a.layout.Bin)
	roles := []catalog.Role{catalog.RoleCore}
	if !a.opts.Minimal {
		roles = append(roles, catalog.RoleExtra)
	}
	if p := a.preflightNetwork(ctx, a.toolsToDownload(roles...), !a.opts.SkipLogin); p != nil {
		return a.fail(p)
	}

	// Step 2: inspect.
	a.step(2, "Looking at what is installed", "look at what is installed")
	in, p := a.inspectTools(ctx, roles...)
	if p != nil {
		return a.fail(p)
	}

	// Step 3: plan and confirm.
	a.step(3, "Making a plan", "make a plan")
	if code, ok := a.resolveTeam(true); !ok {
		return code
	}
	if p := a.inspectKubeconfig(in); p != nil {
		return a.fail(p)
	}
	actions := a.buildPlan(in)
	a.ui.Say("Team: "+a.team, "Namespaces: "+a.stagingNS+" (staging) and "+a.prodNS+" (prod)")
	if len(actions) == 0 {
		a.ui.Item(ui.OK, "Everything is already set up.")
	} else {
		a.ui.Say("kube-onramp will:")
		for i, act := range actions {
			a.ui.Say(fmt.Sprintf("  %d. %s", i+1, act.Text))
		}
		if n := plan.DownloadBytes(actions); n > 0 {
			a.ui.Say("Download size: " + plan.SizeText(n))
		}
	}
	if a.opts.DryRun {
		a.ui.Blank()
		a.ui.Say("This was a dry run (--dry-run). Nothing was changed.")
		return ExitOK
	}
	if len(actions) > 0 && !a.opts.Yes {
		if err := a.ui.WaitEnter("Press Enter to start, or close this window to stop."); err != nil {
			return a.stopped()
		}
	}

	// Step 4: core tools.
	a.step(4, "Installing tools", "download and install the tools")
	installed := map[string]bool{}
	for _, act := range actions {
		t, ok := a.cat.Tool(act.ToolID)
		if act.Kind != plan.InstallTool || !ok || t.Role != catalog.RoleCore {
			continue
		}
		if p := a.installTool(ctx, t); p != nil {
			if interrupted(ctx, p.Cause) {
				return a.stopped()
			}
			return a.fail(p)
		}
		installed[t.ID] = true
	}
	if len(installed) == 0 {
		a.ui.Say("Nothing to install.")
	}
	for _, r := range in.tools {
		if r.Tool.Role != catalog.RoleCore {
			continue
		}
		label := ui.OK
		if installed[r.Tool.ID] {
			label = ui.Fixed
		}
		a.note(label, r.Tool.Name+" "+r.Tool.Version)
	}

	// Step 5: PATH and kubeconfig.
	a.step(5, "Setting up PATH and your Kubernetes settings", "update PATH and the Kubernetes settings")
	done := map[plan.Kind]bool{}
	for _, act := range actions {
		var p *problem.Problem
		switch act.Kind {
		case plan.InstallSelf:
			p = a.installSelf(in.selfPath)
		case plan.UpdatePath:
			p = a.applyPath()
		case plan.WriteKubeconfig, plan.SwitchContext, plan.FixKubeconfigMode:
			p = a.applyKubeconfig(in, act.Kind)
		default:
			continue
		}
		if p != nil {
			return a.fail(p)
		}
		done[act.Kind] = true
	}
	if len(done) == 0 {
		a.ui.Say("Nothing to change.")
	}
	a.recordExisting(in)
	if _, err := a.st.Save(a.layout.StateFile); err != nil {
		a.log.Printf("saving state: %v", err)
	}
	a.noteDone(done[plan.InstallSelf], "kube-onramp in its own folder")
	a.noteDone(done[plan.UpdatePath], "PATH")
	a.noteDone(done[plan.WriteKubeconfig] || done[plan.SwitchContext] || done[plan.FixKubeconfigMode], "Kubernetes settings")

	// Steps 6 and 7: login and access.
	a.step(6, "Checking your login", "log in to the cluster through the browser")
	if a.opts.SkipLogin {
		a.ui.Say("Skipped (--skip-login).")
	} else if p := a.loginStep(ctx); p != nil {
		return a.fail(p)
	}
	if ctx.Err() != nil {
		return a.stopped()
	}
	a.step(7, "Checking your access to the cluster", "check access to the team namespace")
	if a.opts.SkipLogin {
		a.ui.Say("Skipped (--skip-login).")
	} else if p := a.accessStep(ctx); p != nil {
		return a.fail(p)
	}

	// Step 8: extras, then a check in a new terminal, then the summary.
	a.step(8, "Extra tools and final check", "check a new terminal")
	a.installExtras(ctx, actions)
	a.freshShellStep(ctx)
	a.printSummary()
	a.printNextSteps()
	if a.hasFailures() {
		return ExitFail
	}
	return ExitOK
}

func (a *App) noteDone(changed bool, text string) {
	if changed {
		a.note(ui.Fixed, text)
	} else {
		a.note(ui.OK, text)
	}
}

// recordExisting fills state.json for parts that were already fine but are
// not recorded yet (for example after state.json was lost), so reset can
// still find them.
func (a *App) recordExisting(in *inspection) {
	if a.st.Path == nil && in.pathOK {
		a.st.Path = &state.PathChange{Kind: "registry", Dir: a.layout.Bin}
		if in.rcFile != "" {
			a.st.Path = &state.PathChange{Kind: "rcfile", Dir: a.layout.Bin, RCFile: in.rcFile}
		}
	}
	if a.st.Kubeconfig == nil && in.kst.EntriesOK() {
		a.st.Kubeconfig = &state.KubeconfigChange{File: in.kcfg.Path}
	}
}

// installExtras installs optional tools one by one. A failure is only a
// warning: extras never block cluster access.
func (a *App) installExtras(ctx context.Context, actions []plan.Action) {
	for _, act := range actions {
		t, ok := a.cat.Tool(act.ToolID)
		if act.Kind != plan.InstallTool || !ok || t.Role != catalog.RoleExtra {
			continue
		}
		if p := a.installTool(ctx, t); p != nil {
			w := problem.Extra(t.ID, p)
			a.warn(w, t.Name)
			continue
		}
		a.note(ui.Fixed, t.Name+" "+t.Version)
	}
}
