// Package app runs the commands: setup (the full reconcile), check and
// reset. It connects the other packages and decides what users see.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/install"
	"github.com/Sten-Qy-Li/kube-onramp/internal/platform"
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
	"github.com/Sten-Qy-Li/kube-onramp/internal/profile"
	"github.com/Sten-Qy-Li/kube-onramp/internal/report"
	"github.com/Sten-Qy-Li/kube-onramp/internal/state"
	"github.com/Sten-Qy-Li/kube-onramp/internal/ui"
)

// Exit codes.
const (
	ExitOK     = 0 // success, maybe with warnings
	ExitFail   = 1 // a core step failed
	ExitUsage  = 2 // wrong command line, or input needed but none possible
	ExitLocked = 3 // another run is in progress
)

// Options are the command-line settings.
type Options struct {
	Command          string // "setup", "check" or "reset"
	Team             string
	StagingNamespace string
	ProdNamespace    string
	Profile          string
	Minimal          bool
	Yes              bool
	DryRun           bool
	Verbose          bool
	NoColor          bool
	SkipLogin        bool
	Relogin          bool
}

// App holds everything one run needs.
type App struct {
	opts    Options
	version string
	ui      *ui.UI
	log     *report.Logger
	red     *report.Redactor
	layout  platform.Layout
	info    platform.Info
	prof    *profile.Profile
	cat     *catalog.Catalog
	st      *state.State
	http    *http.Client
	now     func() time.Time
	home    string // the user's home folder

	total  int    // number of steps for this command
	trying string // for the error report

	team, stagingNS, prodNS string
	serverVersion           string // from GET /version, "" if unknown
	summary                 []item
	versions                map[string]string // tool id -> version of the own copy, for the report
}

type item struct {
	label ui.Label
	text  string
}

// Run executes one command and returns the exit code.
func Run(ctx context.Context, opts Options, version string) (code int) {
	layout, err := platform.NewLayout()
	if err != nil {
		fmt.Fprintln(os.Stderr, "kube-onramp can't find its folder:", err)
		return ExitFail
	}
	home, _ := os.UserHomeDir()
	red := report.NewRedactor(home, osUserName())
	started := time.Now()
	log, logErr := report.OpenLog(layout.Logs, started, red)
	defer func() { _ = log.Close() }()

	a := &App{
		opts: opts, version: version, log: log, red: red, layout: layout,
		home: home, now: time.Now, http: install.NewHTTPClient(),
		ui:       ui.New(os.Stdout, os.Stdin, log, opts.NoColor, opts.Verbose),
		versions: map[string]string{},
	}
	defer a.keepWindowOpen()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic: %v\n%s", r, debug.Stack())
			code = a.fail(problem.Internal(fmt.Errorf("panic: %v", r)))
		}
	}()

	a.info = platform.Detect()
	log.Printf("kube-onramp %s, command %q, %s, %s/%s", version, opts.Command, a.info.Name, runtime.GOOS, runtime.GOARCH)
	log.Printf("options: %+v", opts)
	log.Printf("home folder: %s", layout.Home)
	if logErr != nil {
		log.Printf("could not open the log file: %v", logErr)
	}

	if opts.Profile == "" {
		opts.Profile = profile.DefaultID
		a.opts.Profile = opts.Profile
	}
	a.prof, err = profile.LoadEmbedded(opts.Profile)
	if errors.Is(err, profile.ErrUnknown) {
		a.ui.Say("There is no profile called \"" + opts.Profile + "\". Leave out --profile to use the default.")
		return ExitUsage
	}
	if err != nil {
		return a.fail(problem.Internal(err))
	}
	if a.cat, err = catalog.Load(); err != nil {
		return a.fail(problem.Internal(err))
	}

	switch opts.Command {
	case "check":
		return a.check(ctx)
	case "reset":
		return a.reset(ctx)
	default:
		return a.setup(ctx)
	}
}

// step prints the step header and remembers it for the error report.
func (a *App) step(n int, text, trying string) {
	a.ui.Step(n, a.total, text)
	a.trying = fmt.Sprintf("%s (step %d of %d)", trying, n, a.total)
}

// note adds a line to the final summary.
func (a *App) note(l ui.Label, text string) {
	a.summary = append(a.summary, item{l, text})
}

// fail shows an error, saves the report, copies it to the clipboard and
// returns ExitFail.
func (a *App) fail(p *problem.Problem) int {
	if p.Trying == "" {
		p.Trying = a.trying
	}
	if p.Trying == "" {
		p.Trying = "start kube-onramp"
	}
	a.ui.Problem(p)
	text := report.Build(p, a.reportInfo(), a.log.Tail(report.ReportLines), a.red)
	path, saveErr := report.Save(a.layout.Logs, a.now(), text)
	// The clipboard only helps a person at a terminal; scripts and CI skip it.
	clipErr := errors.New("no terminal")
	if a.ui.TTY() {
		clipErr = report.CopyToClipboard(text)
	}
	if clipErr != nil {
		a.log.Printf("clipboard: %v", clipErr)
	}
	a.ui.Blank()
	switch {
	case clipErr == nil && saveErr == nil:
		a.ui.Say("The report is copied. Paste it into Slack or into an AI chatbot and ask for help.",
			"The report is also saved here: "+path)
	case saveErr == nil:
		a.ui.Say("Open this file and copy its text into Slack or into an AI chatbot to ask for help:", "  "+path)
	default:
		a.log.Printf("saving the report: %v", saveErr)
		a.ui.Say("The report could not be saved. Here it is:")
		a.ui.Say(strings.Split(strings.TrimRight(text, "\n"), "\n")...)
	}
	return ExitFail
}

// warn shows a warning and adds it to the summary.
func (a *App) warn(p *problem.Problem, summary string) {
	a.ui.Problem(p)
	a.note(ui.Warn, summary+" ("+p.Code+")")
}

func (a *App) reportInfo() report.Info {
	info := report.Info{Version: a.version, OS: a.info.Name, Shell: a.info.Shell}
	if v := a.versions["kubectl"]; v != "" {
		info.Kubectl = "v" + v
	}
	if v := a.versions["kubelogin"]; v != "" {
		info.Kubelogin = "v" + v
	}
	return info
}

// keepWindowOpen waits for Enter when the console window belongs only to
// this process (double-clicked in Explorer), so people can read the result.
func (a *App) keepWindowOpen() {
	if runtime.GOOS == "windows" && platform.OwnsConsole() && a.ui.Interactive() {
		_ = a.ui.WaitEnter("Press Enter to close.")
	}
}

// interrupted reports whether the user pressed Ctrl+C.
func interrupted(ctx context.Context, err error) bool {
	return ctx.Err() != nil && (err == nil || errors.Is(err, context.Canceled) || errors.Is(err, ctx.Err()))
}

func (a *App) stopped() int {
	a.ui.Blank()
	a.ui.Say("Stopped. Nothing is half-done: run kube-onramp again to continue.")
	return ExitFail
}

func (a *App) ownPath(name string) string {
	return filepath.Join(a.layout.Bin, platform.ExeName(name))
}

// tilde shortens a path in the home folder to ~/..., for messages.
func (a *App) tilde(p string) string {
	if a.home != "" {
		if rel, err := filepath.Rel(a.home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return p
}

func osUserName() string {
	name := os.Getenv("USERNAME")
	if name == "" {
		name = os.Getenv("USER")
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:] // DOMAIN\user
	}
	return name
}
