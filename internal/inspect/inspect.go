// Package inspect looks at the machine without changing it: kube-onramp's
// own tool copies, other copies on PATH, and what a new terminal would run.
package inspect

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/fsutil"
	"github.com/Sten-Qy-Li/kube-onramp/internal/pathenv"
	"github.com/Sten-Qy-Li/kube-onramp/internal/plan"
	"github.com/Sten-Qy-Li/kube-onramp/internal/state"
)

// probeTimeout limits each version or identity command.
const probeTimeout = 20 * time.Second

// Copy is one copy of a tool and the result of checks 2-4.
type Copy struct {
	Path         string
	Runs         bool
	RightProduct bool
	Version      string // "" when unknown
	Detail       string // short output or error, for the log
}

// Probe runs a copy's identity and version commands.
func Probe(ctx context.Context, t *catalog.Tool, path string) Copy {
	c := Copy{Path: path}
	idOut, err := run(ctx, path, t.Identity.Command...)
	if err != nil && idOut == "" {
		c.Detail = err.Error()
		return c
	}
	c.RightProduct = t.IsRightProduct(idOut)
	verOut, err := run(ctx, path, t.VersionCommand...)
	if err != nil {
		c.Detail = firstLine(verOut + " " + err.Error())
		return c
	}
	c.Runs = true
	c.Version, _ = t.ParseVersion(verOut)
	c.Detail = firstLine(verOut)
	return c
}

// Tool is the inspection result for one catalog tool.
type Tool struct {
	Tool    *catalog.Tool
	OwnPath string
	Own     *Copy // nil if kube-onramp's copy is missing
	OwnSHA  string
	Status  plan.ToolStatus
	Reason  string // why it is not OK, for the log
	Others  []Copy // other copies found on PATH
}

// InspectTool checks kube-onramp's own copy and the other copies in dirs.
func InspectTool(ctx context.Context, t *catalog.Tool, binDir string, st *state.State, dirs []string) Tool {
	r := Tool{Tool: t, OwnPath: filepath.Join(binDir, t.FileName(runtime.GOOS))}
	r.Status, r.Reason = ownStatus(ctx, &r, st)
	for _, p := range pathenv.FindAll(dirs, t.Binary, runtime.GOOS, os.Getenv("PATHEXT")) {
		if pathenv.SameFile(p, r.OwnPath, runtime.GOOS) {
			continue
		}
		r.Others = append(r.Others, Probe(ctx, t, p))
	}
	return r
}

func ownStatus(ctx context.Context, r *Tool, st *state.State) (plan.ToolStatus, string) {
	if _, err := os.Stat(r.OwnPath); errors.Is(err, fs.ErrNotExist) {
		return plan.ToolMissing, "not installed"
	}
	sum, err := fsutil.FileSHA256(r.OwnPath)
	if err != nil {
		return plan.ToolBroken, "can't read the file: " + err.Error()
	}
	r.OwnSHA = sum
	c := Probe(ctx, r.Tool, r.OwnPath)
	r.Own = &c
	switch {
	case !c.Runs:
		return plan.ToolBroken, "does not run: " + c.Detail
	case !c.RightProduct:
		return plan.ToolBroken, "is not the right program"
	}
	rec, hasRec := st.Tools[r.Tool.ID]
	if hasRec && rec.SHA256 != "" && rec.SHA256 != sum {
		return plan.ToolBroken, "the file changed after it was installed"
	}
	if c.Version == "" && hasRec && rec.SHA256 == sum {
		// Some release builds print no version (kubelogin v1.36.4, D23).
		// The file is the one kube-onramp installed, so its record is exact.
		c.Version = rec.Version
	}
	switch {
	case c.Version == "":
		return plan.ToolBroken, "prints no version and was not installed by kube-onramp"
	case c.Version != r.Tool.Version:
		return plan.ToolOutdated, "version " + c.Version
	}
	return plan.ToolOK, ""
}

// SelfNeedsCopy reports whether the running kube-onramp is not yet (the same
// file as) the copy in binDir.
func SelfNeedsCopy(binDir string) (bool, string, error) {
	exe, err := Executable()
	if err != nil {
		return false, "", err
	}
	own := filepath.Join(binDir, exeName("kube-onramp"))
	if pathenv.SameFile(exe, own, runtime.GOOS) {
		return false, exe, nil
	}
	ownSum, err := fsutil.FileSHA256(own)
	if err != nil {
		return true, exe, nil
	}
	exeSum, err := fsutil.FileSHA256(exe)
	if err != nil {
		return false, exe, err
	}
	return ownSum != exeSum, exe, nil
}

// Executable is the running binary with symlinks resolved.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// NewShell is what a newly opened terminal would see.
type NewShell struct {
	Dirs    []string
	Winners map[string]string // name -> path that runs (or alias text)
}

// LookNewShell computes the PATH of a new terminal and which file wins for
// each name (check 5). On Windows it uses the registry; elsewhere it starts
// the user's shell.
func LookNewShell(ctx context.Context, names []string) (NewShell, error) {
	ns := NewShell{Winners: map[string]string{}}
	if runtime.GOOS == "windows" {
		dirs, err := pathenv.NewShellDirs()
		if err != nil {
			return ns, err
		}
		ns.Dirs = dirs
		for _, n := range names {
			if all := pathenv.FindAll(dirs, n, "windows", os.Getenv("PATHEXT")); len(all) > 0 {
				ns.Winners[n] = all[0]
			}
		}
		return ns, nil
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = pathenv.DefaultShell(runtime.GOOS)
	}
	dirs, found, err := pathenv.NewShellLookup(ctx, shell, names)
	if err != nil {
		return ns, err
	}
	ns.Dirs, ns.Winners = dirs, found
	return ns, nil
}

// MinorSkew returns |minor(a) - minor(b)| for versions like "1.36.6", and
// false if a version can't be read or the major versions differ.
func MinorSkew(a, b string) (int, bool) {
	ma, na, ok1 := majorMinor(a)
	mb, nb, ok2 := majorMinor(b)
	if !ok1 || !ok2 || ma != mb {
		return 0, false
	}
	d := na - nb
	if d < 0 {
		d = -d
	}
	return d, true
}

func majorMinor(v string) (int, int, bool) {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	ma, err1 := strconv.Atoi(parts[0])
	mi, err2 := strconv.Atoi(strings.TrimRight(parts[1], "+"))
	return ma, mi, err1 == nil && err2 == nil
}

func run(ctx context.Context, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // runs tool binaries found on PATH, by design
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func exeName(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}
