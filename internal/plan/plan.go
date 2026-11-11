// Package plan turns what inspect found into a short list of actions in
// plain B1 English. It has no I/O, so it is fully table-tested.
package plan

import (
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
)

// ToolStatus is the state of kube-onramp's own copy of a tool.
type ToolStatus int

// Tool states.
const (
	ToolOK       ToolStatus = iota
	ToolMissing             // no own copy
	ToolOutdated            // own copy has another version
	ToolBroken              // own copy does not run, is the wrong program, or was changed
)

// Kind is the type of an action.
type Kind int

// Action kinds, in the order they run.
const (
	InstallTool Kind = iota
	InstallSelf
	UpdatePath
	WriteKubeconfig
	SwitchContext
	FixKubeconfigMode
)

// Tool is the input for one catalog tool.
type Tool struct {
	ID, Name, Version string
	Status            ToolStatus
	DownloadBytes     int64
}

// Input is everything the plan needs to know.
type Input struct {
	GOOS        string
	ClusterNoun string // "the course cluster"
	Tools       []Tool
	// SelfNeedsCopy is true if kube-onramp itself is not (the same file) in
	// its bin folder yet.
	SelfNeedsCopy bool
	PathOK        bool
	RCFile        string // shown in the PATH action on macOS ("~/.zshrc")
	Kubeconfig    KubeconfigInput
}

// KubeconfigInput describes the kubeconfig.
type KubeconfigInput struct {
	Broken     bool // can't be read; will be replaced (after a backup)
	EntriesOK  bool
	AnyPresent bool // some profile entries exist (update instead of add)
	CurrentOK  bool
	ModeOK     bool
}

// Action is one step of the plan.
type Action struct {
	Kind   Kind
	ToolID string
	Text   string
	Bytes  int64 // download size
}

// Compute returns the actions needed, or none if everything is set up.
func Compute(in Input) []Action {
	var out []Action
	for _, t := range in.Tools {
		var text string
		switch t.Status {
		case ToolOK:
			continue
		case ToolMissing:
			text = "Install " + t.Name + " " + t.Version
		case ToolOutdated:
			text = "Update " + t.Name + " to " + t.Version
		case ToolBroken:
			text = "Repair " + t.Name + " " + t.Version + " (download it again)"
		}
		out = append(out, Action{Kind: InstallTool, ToolID: t.ID, Text: text, Bytes: t.DownloadBytes})
	}
	if in.SelfNeedsCopy {
		out = append(out, Action{Kind: InstallSelf, Text: "Copy kube-onramp into its own folder"})
	}
	if !in.PathOK {
		text := "Add kube-onramp's folder to the start of your PATH (the list of folders where Windows looks for programs)"
		if in.GOOS != "windows" {
			text = "Add kube-onramp's folder to your PATH in " + in.RCFile + " (the list of folders where the terminal looks for programs)"
		}
		out = append(out, Action{Kind: UpdatePath, Text: text})
	}
	noun := in.ClusterNoun
	k := in.Kubeconfig
	switch {
	case k.Broken:
		out = append(out, Action{Kind: WriteKubeconfig, Text: "Replace your Kubernetes settings file, because it can't be read (a backup is made first)"})
	case !k.EntriesOK && k.AnyPresent:
		out = append(out, Action{Kind: WriteKubeconfig, Text: "Update " + noun + " in your Kubernetes settings (a backup is made first)"})
	case !k.EntriesOK:
		out = append(out, Action{Kind: WriteKubeconfig, Text: "Add " + noun + " to your Kubernetes settings (a backup is made first)"})
	case !k.CurrentOK:
		out = append(out, Action{Kind: SwitchContext, Text: "Make " + noun + " the default in your Kubernetes settings (a backup is made first)"})
	}
	if !k.Broken && !k.ModeOK {
		out = append(out, Action{Kind: FixKubeconfigMode, Text: "Make your Kubernetes settings file private (only you can read it)"})
	}
	return out
}

// DownloadBytes is the total download size of the actions.
func DownloadBytes(actions []Action) int64 {
	var n int64
	for _, a := range actions {
		n += a.Bytes
	}
	return n
}

// Has reports whether the plan contains an action of kind k.
func Has(actions []Action, k Kind) bool {
	for _, a := range actions {
		if a.Kind == k {
			return true
		}
	}
	return false
}

// SizeText is a rough download size for people ("about 72 MB").
func SizeText(bytes int64) string {
	return "about " + problem.HumanBytes(bytes)
}
