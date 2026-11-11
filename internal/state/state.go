// Package state reads and writes state.json: what kube-onramp has changed on
// this machine, so that `reset` can undo exactly that.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Sten-Qy-Li/kube-onramp/internal/fsutil"
)

// SchemaVersion is the state format this build writes.
const SchemaVersion = 1

// State is the content of state.json. It must not contain values that change
// on every run (like timestamps), so that a run with nothing to do writes
// nothing.
type State struct {
	SchemaVersion    int                      `json:"schemaVersion"`
	Profile          string                   `json:"profile,omitempty"`
	Team             string                   `json:"team,omitempty"`
	StagingNamespace string                   `json:"stagingNamespace,omitempty"` // only when overridden
	ProdNamespace    string                   `json:"prodNamespace,omitempty"`    // only when overridden
	Tools            map[string]InstalledTool `json:"tools,omitempty"`
	Path             *PathChange              `json:"path,omitempty"`
	Kubeconfig       *KubeconfigChange        `json:"kubeconfig,omitempty"`
}

// InstalledTool is a binary that kube-onramp put into its bin folder.
type InstalledTool struct {
	Version string `json:"version"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"` // of the installed file (not the archive)
}

// PathChange records how PATH was changed.
type PathChange struct {
	Kind string `json:"kind"` // "registry" (Windows) or "rcfile" (macOS, Linux)
	Dir  string `json:"dir"`
	// RCFile is the shell file with the marked block.
	RCFile string `json:"rcFile,omitempty"`
	// RCFileCreated is true if kube-onramp created the rc file.
	RCFileCreated bool `json:"rcFileCreated,omitempty"`
	// ValueCreated is true if the user PATH value did not exist before.
	ValueCreated bool `json:"valueCreated,omitempty"`
}

// KubeconfigChange records the kubeconfig that kube-onramp wrote.
type KubeconfigChange struct {
	File string `json:"file"`
	// Created is true if the file did not exist before.
	Created bool `json:"created,omitempty"`
	// PreviousContext is the current context before kube-onramp switched it.
	PreviousContext string `json:"previousContext,omitempty"`
}

// Load reads state.json. A missing file gives an empty state. A broken file
// also gives an empty state, plus an error to log.
func Load(path string) (*State, error) {
	s := &State{SchemaVersion: SchemaVersion}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return &State{SchemaVersion: SchemaVersion}, fmt.Errorf("state file is broken, starting fresh: %w", err)
	}
	return s, nil
}

// Save writes the state atomically, but only if the content changed.
func (s *State) Save(path string) (changed bool, err error) {
	s.SchemaVersion = SchemaVersion
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	return true, fsutil.WriteFileAtomic(path, data, 0o600)
}
