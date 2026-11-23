package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
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
