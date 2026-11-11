package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Sten-Qy-Li/kube-onramp/internal/fsutil"
	"github.com/Sten-Qy-Li/kube-onramp/internal/kubeconfig"
	"github.com/Sten-Qy-Li/kube-onramp/internal/pathenv"
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
	"github.com/Sten-Qy-Li/kube-onramp/internal/ui"
)

// removal is one thing reset removes.
type removal struct {
	text string
	run  func() error
}

// reset removes exactly what kube-onramp added. It does not restore an old
// backup (that would lose later changes), and it keeps the backups folder.
func (a *App) reset(ctx context.Context) int {
	_ = ctx
	if !a.opts.Yes && !a.ui.Interactive() {
		a.ui.Say("kube-onramp needs you to press Enter once, but it can't read from this window.",
			"Run it in a normal terminal window, or add --yes.")
		return ExitUsage
	}
	a.ui.Title("Removing kube-onramp from this computer")
	a.trying = "remove kube-onramp (reset)"
	lock, code, ok := a.preflightLocal(false)
	if !ok {
		return code
	}
	defer lock.Release()

	var items []removal
	if r, p := a.kubeconfigRemoval(); p != nil {
		return a.fail(p)
	} else if r != nil {
		items = append(items, *r)
	}
	if r, p := a.pathRemoval(); p != nil {
		return a.fail(p)
	} else if r != nil {
		items = append(items, *r)
	}
	if exists(a.layout.Cache) {
		items = append(items, removal{"Delete kube-onramp's login cache", func() error { return os.RemoveAll(a.layout.Cache) }})
	}
	if exists(a.layout.Bin) {
		items = append(items, removal{"Delete kube-onramp's programs in " + a.tilde(a.layout.Bin), func() error {
			_ = os.RemoveAll(a.layout.Tmp)
			_ = os.Remove(a.layout.StateFile)
			return removeBinDir(a.layout.Bin, a.log.Printf)
		}})
	} else if exists(a.layout.StateFile) {
		items = append(items, removal{"Delete kube-onramp's notes about this computer", func() error {
			_ = os.RemoveAll(a.layout.Tmp)
			return os.Remove(a.layout.StateFile)
		}})
	}

	if len(items) == 0 {
		a.ui.Item(ui.OK, "Nothing to remove. kube-onramp is not set up on this computer.")
		return ExitOK
	}
	a.ui.Say("kube-onramp will:")
	for _, it := range items {
		a.ui.Say("  - " + it.text)
	}
	a.ui.Say("Other programs are not touched. Your backups stay in: " + a.layout.Backups)
	if !a.opts.Yes {
		if err := a.ui.WaitEnter("Press Enter to remove, or close this window to stop."); err != nil {
			return a.stopped()
		}
	}
	a.ui.Blank()
	for _, it := range items {
		if err := it.run(); err != nil {
			a.ui.Item(ui.Fail, it.text)
			return a.fail(problem.Install("the removal", err))
		}
		a.ui.Item(ui.Fixed, it.text)
	}
	a.ui.Blank()
	a.ui.Say("kube-onramp is removed. Open a new terminal to see the change.",
		"To set it up again, use the install line from the README.")
	return ExitOK
}

// kubeconfigRemoval removes only the profile entries.
func (a *App) kubeconfigRemoval() (*removal, *problem.Problem) {
	target := kubeconfig.TargetFile()
	prev, created := "", false
	if rec := a.st.Kubeconfig; rec != nil {
		if rec.File != "" {
			target = rec.File
		}
		prev, created = rec.PreviousContext, rec.Created
	}
	f, err := kubeconfig.Read(target)
	if err != nil {
		return nil, problem.Kubeconfig(target, err)
	}
	if f.Broken {
		a.log.Printf("kubeconfig %s can't be read (%s); leaving it alone", target, f.BrokenReason)
		return nil, nil
	}
	// Only the entry names matter for removal.
	want, err := kubeconfig.Desired(a.prof, "x", "x", "x", "x")
	if err != nil {
		return nil, problem.Internal(err)
	}
	if st := kubeconfig.Check(f, want); !st.AnyPresent && !st.CurrentOK {
		return nil, nil
	}
	return &removal{"Remove " + a.prof.ClusterNoun + " from your Kubernetes settings (a backup is made first)", func() error {
		if _, err := kubeconfig.Backup(f.Raw, a.layout.Backups, a.now()); err != nil {
			return err
		}
		_, empty := kubeconfig.Remove(f.Config, want, prev)
		path := target
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			path = resolved
		}
		if empty && created {
			return os.Remove(path)
		}
		data, err := kubeconfig.Encode(f.Config)
		if err != nil {
			return err
		}
		return kubeconfig.WriteFile(path, data)
	}}, nil
}

// pathRemoval removes the PATH entry (Windows) or the rc block (macOS).
func (a *App) pathRemoval() (*removal, *problem.Problem) {
	bin := a.layout.Bin
	if a.st.Path != nil && a.st.Path.Dir != "" {
		bin = a.st.Path.Dir
	}
	if runtime.GOOS == "windows" {
		up, err := pathenv.ReadUserPath()
		if err != nil {
			return nil, problem.Path(err)
		}
		value, changed := pathenv.RemoveDir(up.Value, bin, pathenv.Expand)
		if !changed {
			return nil, nil
		}
		created := a.st.Path != nil && a.st.Path.ValueCreated
		return &removal{"Remove kube-onramp's folder from your PATH", func() error {
			defer pathenv.Broadcast()
			if value == "" && created {
				return pathenv.DeleteUserPath()
			}
			up.Value = value
			return pathenv.WriteUserPath(up)
		}}, nil
	}
	rc := a.rcFile()
	target := rc
	if resolved, err := filepath.EvalSymlinks(rc); err == nil {
		target = resolved
	}
	content, err := os.ReadFile(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, problem.Path(err)
	}
	next, changed := pathenv.RemoveBlock(string(content))
	if !changed {
		return nil, nil
	}
	created := a.st.Path != nil && a.st.Path.RCFileCreated
	return &removal{"Remove the kube-onramp lines from " + a.tilde(rc), func() error {
		if next == "" && created {
			return os.Remove(target)
		}
		mode := os.FileMode(0o644)
		if fi, err := os.Stat(target); err == nil {
			mode = fi.Mode().Perm()
		}
		return fsutil.WriteFileAtomic(target, []byte(next), mode)
	}}, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
