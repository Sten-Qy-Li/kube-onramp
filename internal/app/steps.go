package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Sten-Qy-Li/kube-onramp/internal/access"
	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/fsutil"
	"github.com/Sten-Qy-Li/kube-onramp/internal/inspect"
	"github.com/Sten-Qy-Li/kube-onramp/internal/install"
	"github.com/Sten-Qy-Li/kube-onramp/internal/kubeconfig"
	"github.com/Sten-Qy-Li/kube-onramp/internal/login"
	"github.com/Sten-Qy-Li/kube-onramp/internal/pathenv"
	"github.com/Sten-Qy-Li/kube-onramp/internal/plan"
	"github.com/Sten-Qy-Li/kube-onramp/internal/platform"
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
	"github.com/Sten-Qy-Li/kube-onramp/internal/profile"
	"github.com/Sten-Qy-Li/kube-onramp/internal/state"
	"github.com/Sten-Qy-Li/kube-onramp/internal/ui"
)

// newShellNames are checked in a new terminal (check 5 and step 8).
var newShellNames = []string{"kubectl", "kubectl-oidc_login", "kube-onramp"}

// inspection is everything step 2 found.
type inspection struct {
	tools         []inspect.Tool
	selfNeedsCopy bool
	selfPath      string
	pathOK        bool
	rcFile        string // macOS/Linux
	kcfg          *kubeconfig.File
	want          *kubeconfig.Want
	kst           kubeconfig.Status
}

// inspectTools runs the five checks for every tool and looks at PATH.
func (a *App) inspectTools(ctx context.Context, roles ...catalog.Role) (*inspection, *problem.Problem) {
	in := &inspection{}
	dirs := pathenv.SplitList(os.Getenv("PATH"), runtime.GOOS)
	if ns, err := inspect.LookNewShell(ctx, newShellNames); err != nil {
		a.log.Printf("new-shell lookup: %v", err)
	} else {
		dirs = append(dirs, ns.Dirs...)
		a.log.Printf("new shell PATH: %s", strings.Join(ns.Dirs, string(os.PathListSeparator)))
	}
	for _, role := range roles {
		for _, t := range a.cat.ByRole(role) {
			r := inspect.InspectTool(ctx, t, a.layout.Bin, a.st, dirs)
			in.tools = append(in.tools, r)
			a.describeTool(r)
		}
	}

	var err error
	in.selfNeedsCopy, in.selfPath, err = inspect.SelfNeedsCopy(a.layout.Bin)
	if err != nil {
		a.log.Printf("self check: %v", err)
	}

	in.pathOK, in.rcFile, err = a.pathOK()
	if err != nil {
		return nil, problem.Path(err)
	}
	if in.pathOK {
		a.ui.Say("PATH: kube-onramp's folder comes first.")
	} else {
		a.ui.Say("PATH: kube-onramp's folder is not set up yet.")
	}
	return in, nil
}

func (a *App) describeTool(r inspect.Tool) {
	t := r.Tool
	switch r.Status {
	case plan.ToolOK:
		a.versions[t.ID] = t.Version
		a.ui.Say(t.Name + ": installed (" + t.Version + ")")
	case plan.ToolMissing:
		a.ui.Say(t.Name + ": not installed yet")
	case plan.ToolOutdated:
		a.versions[t.ID] = r.Own.Version
		a.ui.Say(t.Name + ": an older version is installed (" + r.Own.Version + ")")
	case plan.ToolBroken:
		a.ui.Say(t.Name + ": kube-onramp's copy is broken and will be downloaded again")
	}
	a.log.Printf("%s: own copy %s: %s", t.ID, r.OwnPath, r.Reason)
	for _, o := range r.Others {
		v := o.Version
		if v == "" {
			v = "version unknown"
		}
		if !o.RightProduct {
			v += ", a different program with the same name"
		}
		a.ui.Say("  Another copy (kube-onramp leaves it alone): " + o.Path + " (" + v + ")")
		a.log.Printf("  other copy %s: runs=%v right=%v version=%q detail=%q", o.Path, o.Runs, o.RightProduct, o.Version, o.Detail)
	}
}

// pathOK reports whether PATH already has kube-onramp's bin folder first.
func (a *App) pathOK() (bool, string, error) {
	if runtime.GOOS == "windows" {
		up, err := pathenv.ReadUserPath()
		if err != nil {
			return false, "", err
		}
		_, changed := pathenv.EnsureFirst(up.Value, a.layout.Bin, pathenv.Expand)
		return !changed, "", nil
	}
	rc := a.rcFile()
	content, err := os.ReadFile(rc)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, rc, err
	}
	_, changed := pathenv.EnsureBlock(string(content), pathenv.Block(a.layout.Bin, a.home))
	return !changed, rc, nil
}

func (a *App) rcFile() string {
	if a.st != nil && a.st.Path != nil && a.st.Path.RCFile != "" {
		return a.st.Path.RCFile
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = pathenv.DefaultShell(runtime.GOOS)
	}
	return pathenv.RCFile(shell, a.home, os.Getenv("ZDOTDIR"), runtime.GOOS)
}

// resolveTeam finds the team name: --team, then state.json, then one
// question. It returns false with an exit code when it can't.
func (a *App) resolveTeam(canAsk bool) (int, bool) {
	team := ""
	switch {
	case a.opts.Team != "":
		t, err := a.prof.NormalizeTeam(a.opts.Team)
		if err != nil {
			a.ui.Say(err.Error(), "Example: kube-onramp --team "+a.prof.TeamExample)
			return ExitUsage, false
		}
		team = t
		if t != a.st.Team {
			// A new team: forget namespace overrides that belonged to the old one.
			a.st.StagingNamespace, a.st.ProdNamespace = "", ""
		}
	case a.st.Team != "":
		team = a.st.Team
	case canAsk && a.ui.Interactive():
		a.ui.Say("kube-onramp needs your team name, as your course uses it. Example: " + a.prof.TeamExample)
		for i := 0; i < 3 && team == ""; i++ {
			answer, err := a.ui.Ask("Your team name:")
			if err != nil {
				return ExitUsage, false
			}
			t, err := a.prof.NormalizeTeam(answer)
			if err != nil {
				a.ui.Say(err.Error())
				continue
			}
			team = t
		}
		if team == "" {
			a.ui.Say("No valid team name. Run again with: kube-onramp --team <your-team>")
			return ExitUsage, false
		}
	default:
		a.ui.Say("kube-onramp needs your team name.",
			"Run again with: kube-onramp --team <your-team>   (example: "+a.prof.TeamExample+")")
		return ExitUsage, false
	}
	a.team = team
	a.st.Team = team
	a.st.Profile = a.prof.ID

	for _, o := range []struct {
		flag  string
		saved *string
		tmpl  func(string) string
		out   *string
	}{
		{a.opts.StagingNamespace, &a.st.StagingNamespace, a.prof.StagingNamespace, &a.stagingNS},
		{a.opts.ProdNamespace, &a.st.ProdNamespace, a.prof.ProdNamespace, &a.prodNS},
	} {
		if o.flag != "" {
			if err := profile.ValidateNamespace(o.flag); err != nil {
				a.ui.Say(err.Error())
				return ExitUsage, false
			}
			*o.saved = o.flag
		}
		*o.out = *o.saved
		if *o.out == "" {
			*o.out = o.tmpl(team)
		}
	}
	return 0, true
}

// inspectKubeconfig reads the kubeconfig and compares it with the profile.
func (a *App) inspectKubeconfig(in *inspection) *problem.Problem {
	target := kubeconfig.TargetFile()
	want, err := kubeconfig.Desired(a.prof, a.stagingNS, a.prodNS, a.ownPath("kubectl-oidc_login"), a.layout.OIDCCache)
	if err != nil {
		return problem.Internal(err)
	}
	f, err := kubeconfig.Read(target)
	if err != nil {
		return problem.Kubeconfig(target, err)
	}
	in.kcfg, in.want = f, want
	in.kst = kubeconfig.Check(f, want)
	a.log.Printf("kubeconfig %s: exists=%v broken=%v (%s) status=%+v", target, f.Exists, f.Broken, f.BrokenReason, in.kst)
	return nil
}

// buildPlan turns the inspection into actions.
func (a *App) buildPlan(in *inspection) []plan.Action {
	pi := plan.Input{
		GOOS:          runtime.GOOS,
		ClusterNoun:   a.prof.ClusterNoun,
		SelfNeedsCopy: in.selfNeedsCopy,
		PathOK:        in.pathOK,
		RCFile:        a.tilde(in.rcFile),
		Kubeconfig: plan.KubeconfigInput{
			Broken:     in.kcfg.Broken,
			EntriesOK:  in.kst.EntriesOK(),
			AnyPresent: in.kst.AnyPresent,
			CurrentOK:  in.kst.CurrentOK,
			ModeOK:     in.kst.ModeOK,
		},
	}
	for _, r := range in.tools {
		art, _ := r.Tool.Artifact(runtime.GOOS, runtime.GOARCH)
		pi.Tools = append(pi.Tools, plan.Tool{ID: r.Tool.ID, Name: r.Tool.Name, Version: r.Tool.Version, Status: r.Status, DownloadBytes: art.Size})
	}
	return plan.Compute(pi)
}

// installTool downloads, verifies and installs one tool.
func (a *App) installTool(ctx context.Context, t *catalog.Tool) *problem.Problem {
	art, ok := t.Artifact(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return problem.PreOS(a.info.Name, "a system that "+t.Name+" supports")
	}
	label := "Downloading " + t.Name + " " + t.Version + ":"
	dl, err := install.Download(ctx, a.http, art.URL, art.SHA256, art.Size, a.layout.Tmp, t.ID+"-"+t.Version,
		func(done, total int64) { a.ui.Progress(label, done, total) })
	if err != nil {
		var ce *install.ChecksumError
		if errors.As(err, &ce) {
			return problem.SHA(t.Name, err)
		}
		return problem.Download(t.Name, err)
	}
	defer func() { _ = os.Remove(dl) }()
	tmp := filepath.Join(a.layout.Tmp, t.FileName(runtime.GOOS)+".new")
	if err := install.Extract(dl, art, tmp); err != nil {
		return problem.Extract(t.Name, err)
	}
	own := filepath.Join(a.layout.Bin, t.FileName(runtime.GOOS))
	if err := install.Place(tmp, own); err != nil {
		return problem.Install(t.Name, err)
	}
	// The download already matched its pinned SHA-256. Here we only check that
	// it starts and is the right program. A missing version is accepted
	// (kubelogin v1.36.4 release builds print none, D23); a wrong one is not.
	c := inspect.Probe(ctx, t, own)
	if !c.Runs || !c.RightProduct || (c.Version != "" && c.Version != t.Version) {
		return problem.Install(t.Name, errors.New("the new copy does not start correctly: "+c.Detail))
	}
	sum, err := fsutil.FileSHA256(own)
	if err != nil {
		return problem.Install(t.Name, err)
	}
	a.st.Tools[t.ID] = state.InstalledTool{Version: t.Version, File: filepath.Base(own), SHA256: sum}
	if _, err := a.st.Save(a.layout.StateFile); err != nil {
		a.log.Printf("saving state: %v", err)
	}
	a.versions[t.ID] = t.Version
	a.ui.Item(ui.Fixed, "Installed "+t.Name+" "+t.Version)
	return nil
}

// installSelf copies the running kube-onramp into its bin folder.
func (a *App) installSelf(selfPath string) *problem.Problem {
	tmp := filepath.Join(a.layout.Tmp, platform.ExeName("kube-onramp.new"))
	if err := fsutil.CopyFile(selfPath, tmp, 0o755); err != nil {
		return problem.Install("kube-onramp", err)
	}
	if err := install.Place(tmp, a.ownPath("kube-onramp")); err != nil {
		return problem.Install("kube-onramp", err)
	}
	a.ui.Item(ui.Fixed, "Copied kube-onramp into "+a.tilde(a.layout.Bin))
	return nil
}

// applyPath puts the bin folder first on PATH for new terminals.
func (a *App) applyPath() *problem.Problem {
	bin := a.layout.Bin
	if runtime.GOOS == "windows" {
		up, err := pathenv.ReadUserPath()
		if err != nil {
			return problem.Path(err)
		}
		value, changed := pathenv.EnsureFirst(up.Value, bin, pathenv.Expand)
		created := !up.Exists || (a.st.Path != nil && a.st.Path.ValueCreated)
		if changed {
			next := up
			next.Value = value
			if !up.Exists {
				next.Expand = true
			}
			if err := pathenv.WriteUserPath(next); err != nil {
				return problem.Path(err)
			}
			pathenv.Broadcast()
		}
		a.st.Path = &state.PathChange{Kind: "registry", Dir: bin, ValueCreated: created}
	} else {
		rc := a.rcFile()
		target := rc
		if resolved, err := filepath.EvalSymlinks(rc); err == nil {
			target = resolved // keep a symlinked rc file (dotfiles) a symlink
		}
		content, err := os.ReadFile(target)
		exists := err == nil
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return problem.Path(err)
		}
		mode := os.FileMode(0o644)
		if fi, err := os.Stat(target); err == nil {
			mode = fi.Mode().Perm()
		}
		next, changed := pathenv.EnsureBlock(string(content), pathenv.Block(bin, a.home))
		if changed {
			if err := fsutil.WriteFileAtomic(target, []byte(next), mode); err != nil {
				return problem.Path(err)
			}
		}
		created := !exists || (a.st.Path != nil && a.st.Path.RCFileCreated)
		a.st.Path = &state.PathChange{Kind: "rcfile", Dir: bin, RCFile: rc, RCFileCreated: created}
	}
	// This process (and the programs it starts) should find the new tools too.
	sep := string(os.PathListSeparator)
	if cur := os.Getenv("PATH"); !strings.HasPrefix(cur, bin+sep) {
		_ = os.Setenv("PATH", bin+sep+cur)
	}
	a.ui.Item(ui.Fixed, "PATH: kube-onramp's folder now comes first in new terminals")
	return nil
}

// applyKubeconfig writes the profile entries (after a backup).
func (a *App) applyKubeconfig(in *inspection, kind plan.Kind) *problem.Problem {
	f := in.kcfg
	target := f.Path
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	if kind == plan.FixKubeconfigMode {
		if err := kubeconfig.FixMode(target); err != nil {
			return problem.Kubeconfig(f.Path, err)
		}
		a.ui.Item(ui.Fixed, "Your Kubernetes settings file is now private (mode 600)")
		return nil
	}
	backup := ""
	if f.Exists {
		var err error
		if backup, err = kubeconfig.Backup(f.Raw, a.layout.Backups, a.now()); err != nil {
			return problem.Kubeconfig(f.Path, err)
		}
	}
	cfg, prev := kubeconfig.Apply(f.Config, in.want)
	data, err := kubeconfig.Encode(cfg)
	if err != nil {
		return problem.Internal(err)
	}
	if err := kubeconfig.WriteFile(target, data); err != nil {
		return problem.Kubeconfig(f.Path, err)
	}
	rec := &state.KubeconfigChange{File: f.Path, Created: !f.Exists}
	if old := a.st.Kubeconfig; old != nil && old.File == f.Path {
		rec.Created = rec.Created || old.Created
		if prev == "" {
			prev = old.PreviousContext
		}
	}
	rec.PreviousContext = prev
	a.st.Kubeconfig = rec

	switch {
	case f.Broken:
		a.ui.Item(ui.Fixed, "Made a new Kubernetes settings file: "+f.Path)
		a.ui.Say("Your old file could not be read. It is saved here:", "  "+backup)
	case backup != "":
		a.ui.Item(ui.Fixed, "Updated your Kubernetes settings: "+f.Path)
		a.ui.Say("A copy of the old file is here: " + backup)
	default:
		a.ui.Item(ui.Fixed, "Made your Kubernetes settings file: "+f.Path)
	}
	in.kcfg.Broken = false
	return nil
}

// loginStep checks the login and opens the browser if needed.
func (a *App) loginStep(ctx context.Context) *problem.Problem {
	a.ui.Say("If a browser window opens, log in with "+a.prof.LoginWording+",", "then come back to this window.")
	if a.opts.Relogin {
		if err := login.ClearTokenCache(a.layout.OIDCCache); err != nil {
			a.log.Printf("clear token cache: %v", err)
		}
	}
	shown := false
	opts := login.Options{
		Kubectl: a.ownPath("kubectl"),
		Context: a.prof.Kubeconfig.Contexts.Staging,
		Timeout: login.Timeout,
		Log:     a.log.Printf,
		OnURL: func(u string) {
			if !shown {
				a.ui.Blank()
				a.ui.Say("Open this link in your browser:", "  "+u)
				shown = true
			}
		},
	}
	res, kind, err := login.WhoAmI(ctx, opts)
	if kind == login.Rejected {
		a.log.Printf("login rejected, clearing the token cache and trying once more: %v", err)
		_ = login.ClearTokenCache(a.layout.OIDCCache)
		res, kind, err = login.WhoAmI(ctx, opts)
	}
	switch kind {
	case login.OK:
	case login.TimedOut:
		p := problem.LoginTimeout()
		p.Cause = err
		return p
	case login.Rejected:
		return problem.LoginRejected(err)
	case login.Ports:
		p := problem.PrePorts()
		p.Cause = err
		return p
	case login.Network:
		return problem.ClusterUnreachable(err)
	default:
		if interrupted(ctx, err) {
			return nil
		}
		return problem.LoginFailed(err)
	}
	// Hide the user name in logs and reports from now on (D: privacy).
	a.red.AddClusterUser(res.Username)
	a.ui.Item(ui.OK, "You are logged in as "+res.Username)
	a.ui.Say("Is this you? If not, log out of the wrong account in your browser,",
		"then run: kube-onramp check --relogin")
	a.note(ui.OK, "Login")
	return nil
}

// accessStep runs the profile's can-i checks.
func (a *App) accessStep(ctx context.Context) *problem.Problem {
	kubectl := a.ownPath("kubectl")
	checks := a.prof.AccessChecks
	staging := access.Run(ctx, kubectl, a.prof.Kubeconfig.Contexts.Staging, a.stagingNS, checks.Staging.Required)
	for _, r := range staging {
		if r.Err != nil {
			return problem.ClusterUnreachable(r.Err)
		}
		if !r.Allowed {
			return problem.AccessForbidden(a.stagingNS)
		}
	}
	a.ui.Item(ui.OK, "You can use the namespace "+a.stagingNS)
	a.note(ui.OK, "Access to "+a.stagingNS)
	var can, cannot []string
	for _, r := range access.Run(ctx, kubectl, a.prof.Kubeconfig.Contexts.Staging, a.stagingNS, checks.Staging.Informational) {
		switch {
		case r.Err != nil:
			a.log.Printf("can-i %s: %v", r.Text(), r.Err)
		case r.Allowed:
			can = append(can, r.Text())
		default:
			cannot = append(cannot, r.Text())
		}
	}
	if len(can) > 0 {
		a.ui.Say("  You can: " + strings.Join(can, ", "))
	}
	if len(cannot) > 0 {
		a.ui.Say("  You cannot: " + strings.Join(cannot, ", "))
	}

	prodOK := len(checks.Prod.Required) > 0
	for _, r := range access.Run(ctx, kubectl, a.prof.Kubeconfig.Contexts.Prod, a.prodNS, checks.Prod.Required) {
		if r.Err != nil || !r.Allowed {
			prodOK = false
			a.log.Printf("prod can-i %s: allowed=%v err=%v", r.Text(), r.Allowed, r.Err)
		}
	}
	if prodOK {
		a.ui.Item(ui.OK, "You can use the namespace "+a.prodNS)
		a.note(ui.OK, "Access to "+a.prodNS)
	} else if len(checks.Prod.Required) > 0 {
		a.warn(problem.ProdNotYet(a.prodNS), "Access to "+a.prodNS)
	}

	if own, ok := a.cat.Tool("kubectl"); ok && a.serverVersion != "" {
		if d, ok := inspect.MinorSkew(own.Version, a.serverVersion); ok && d > 1 {
			a.warn(problem.Skew(own.Version, a.serverVersion), "kubectl version")
		}
	}
	return nil
}

// freshShellStep checks what a new terminal runs (step 8).
func (a *App) freshShellStep(ctx context.Context) {
	ns, err := inspect.LookNewShell(ctx, newShellNames)
	if err != nil {
		a.log.Printf("new-shell check: %v", err)
		a.ui.Item(ui.Warn, "Could not check a new terminal. Open one and try: kubectl version --client")
		a.note(ui.Warn, "New terminal not checked")
		return
	}
	for _, name := range newShellNames {
		own := a.ownPath(name)
		winner := ns.Winners[name]
		switch {
		case winner != "" && pathenv.SameFile(winner, own, runtime.GOOS):
			a.ui.Item(ui.OK, "A new terminal runs kube-onramp's "+name)
		case winner == "":
			a.ui.Item(ui.Warn, "A new terminal does not find "+name+" yet")
			a.note(ui.Warn, "New terminal: "+name+" not found")
		case name == "kubectl-oidc_login":
			// The kubeconfig calls kube-onramp's copy by its full path.
			a.ui.Item(ui.OK, "A new terminal finds another "+name+", but your settings use kube-onramp's copy directly")
		default:
			a.shadowWarning(ctx, name, winner)
		}
	}
}

func (a *App) shadowWarning(ctx context.Context, name, winner string) {
	acceptable := false
	if t, ok := a.cat.Tool(name); ok {
		c := inspect.Probe(ctx, t, winner)
		ref := a.serverVersion
		if ref == "" {
			ref = t.Version
		}
		if d, ok := inspect.MinorSkew(c.Version, ref); ok && c.RightProduct && d <= 1 {
			acceptable = true
		}
	}
	hint := "Remove or update the other copy, or ask a TA for help."
	if strings.Contains(strings.ToLower(winner), "docker") {
		hint = "Update Docker Desktop (it brings its own kubectl), or ask a TA for help."
	}
	if runtime.GOOS == "windows" {
		hint += " It is on the system PATH, which kube-onramp can't change without admin rights."
	}
	a.warn(problem.Shadow(name, winner, acceptable, hint), "New terminal: "+name)
}

// printSummary prints the summary and the next steps.
func (a *App) printSummary() {
	a.ui.Title("Summary")
	for _, it := range a.summary {
		a.ui.Item(it.label, it.text)
	}
}

func (a *App) printNextSteps() {
	ctx := a.prof.Kubeconfig.Contexts
	a.ui.Title("What to do now")
	a.ui.Say(
		"1. Open a NEW terminal window.",
		"2. Try: kubectl get pods",
		"3. Staging is the default. To use prod: kubectl config use-context "+ctx.Prod,
		"   To go back to staging: kubectl config use-context "+ctx.Staging,
		"4. Before a presentation, run: kube-onramp check",
	)
	if len(a.prof.HelpLinks) > 0 {
		a.ui.Title("Help")
		for _, l := range a.prof.HelpLinks {
			a.ui.Say(l.Name + ": " + l.URL)
		}
	}
}

func (a *App) hasFailures() bool {
	for _, it := range a.summary {
		if it.label == ui.Fail {
			return true
		}
	}
	return false
}
