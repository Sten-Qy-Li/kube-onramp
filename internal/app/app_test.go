package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/platform"
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
)

func TestSplitToolProblems(t *testing.T) {
	kubectl := &catalog.Tool{ID: "kubectl", Role: catalog.RoleCore}
	helm := &catalog.Tool{ID: "helm", Role: catalog.RoleExtra}
	trivy := &catalog.Tool{ID: "trivy", Role: catalog.RoleExtra}
	netErr := errors.New("timeout")
	helmNet := problem.PreNet("get.helm.sh", netErr)
	trivyOS := problem.PreOS("macOS 12", "macOS 13")
	coreNet := problem.PreNet("dl.k8s.io", netErr)
	clusterNet := problem.PreNet("kubernetes.example", netErr)

	tests := []struct {
		name     string
		in       []toolProblem
		wantStop *problem.Problem
		wantSkip []string
	}{
		{"nothing wrong", nil, nil, nil},
		{"only extras: skip them, don't stop", []toolProblem{{helm, helmNet}, {trivy, trivyOS}}, nil, []string{"helm", "trivy"}},
		{"a core tool stops the run", []toolProblem{{helm, helmNet}, {kubectl, coreNet}}, coreNet, nil},
		{"a cluster check stops the run", []toolProblem{{nil, clusterNet}, {helm, helmNet}}, clusterNet, nil},
	}
	for _, tt := range tests {
		stop, skip := splitToolProblems(tt.in)
		if stop != tt.wantStop {
			t.Errorf("%s: stop = %v, want %v", tt.name, stop, tt.wantStop)
		}
		var ids []string
		for _, s := range skip {
			ids = append(ids, s.tool.ID)
		}
		if strings.Join(ids, ",") != strings.Join(tt.wantSkip, ",") {
			t.Errorf("%s: skip = %v, want %v", tt.name, ids, tt.wantSkip)
		}
	}
}

func TestProdProblem(t *testing.T) {
	if p := prodProblem(true, "ns-x-prod"); p.Code != "E-PROD-FORBIDDEN" || p.IsWarning() {
		t.Errorf("required: got %s, want the error E-PROD-FORBIDDEN", p.Code)
	}
	if p := prodProblem(false, "ns-x-prod"); p.Code != "W-PROD-NOT-YET" || !p.IsWarning() {
		t.Errorf("not required: got %s, want the warning W-PROD-NOT-YET", p.Code)
	}
}

func TestShellNamesComeFromTheCatalog(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	core := shellNames(c.ByRole(catalog.RoleCore))
	if got, want := strings.Join(core, ","), "kubectl,kubectl-oidc_login,kube-onramp"; got != want {
		t.Errorf("core names = %s, want %s", got, want)
	}
	all := shellNames(append(c.ByRole(catalog.RoleCore), c.ByRole(catalog.RoleExtra)...))
	if got, want := strings.Join(all, ","), "kubectl,kubectl-oidc_login,helm,kustomize,kubeconform,trivy,k9s,oha,kube-onramp"; got != want {
		t.Errorf("all names = %s, want %s", got, want)
	}
}

func TestToolSystemProblem(t *testing.T) {
	art := catalog.Artifact{MinOS: "10.0"}
	withNeeds := art
	withNeeds.Needs = &catalog.SystemFile{File: "vcruntime140.dll", Package: "Microsoft Visual C++ Redistributable", URL: "https://aka.ms/vc14/vc_redist.x64.exe"}
	oha := &catalog.Tool{ID: "oha", Name: "oha", Version: "1.17.0", Role: catalog.RoleExtra, Platforms: map[string]catalog.Artifact{
		"windows/amd64": withNeeds,
		"darwin/amd64":  {MinOS: "10.12"},
	}}
	win11 := platform.Info{Name: "Windows 11", Version: "10.0.26100"}
	oldMac := platform.Info{Name: "macOS 10.11", Version: "10.11.6"}
	missing := func(string) bool { return false }
	present := func(string) bool { return true }

	tests := []struct {
		name     string
		goos     string
		info     platform.Info
		has      func(string) bool
		wantCode string
	}{
		{"DLL present", "windows", win11, present, ""},
		{"DLL missing", "windows", win11, missing, "E-PRE-RUNTIME"},
		{"no download for this system", "linux", win11, present, "E-PRE-OS"},
		{"system too old", "darwin", oldMac, present, "E-PRE-OS"},
		{"needs is not looked at on macOS", "darwin", platform.Info{Name: "macOS 13", Version: "13.7.8"}, missing, ""},
	}
	for _, tt := range tests {
		p := toolSystemProblem(oha, tt.goos, "amd64", tt.info, tt.has)
		got := ""
		if p != nil {
			got = p.Code
		}
		if got != tt.wantCode {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.wantCode)
		}
	}
	// Skipped, not stopped: oha is an extra (D45).
	p := toolSystemProblem(oha, "windows", "amd64", win11, missing)
	if stop, skip := splitToolProblems([]toolProblem{{oha, p}}); stop != nil || len(skip) != 1 {
		t.Errorf("a missing DLL for an extra must skip it: stop=%v skip=%d", stop, len(skip))
	}
}
