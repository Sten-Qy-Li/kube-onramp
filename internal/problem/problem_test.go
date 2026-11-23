package problem

import (
	"errors"
	"strings"
	"testing"
)

func TestHumanBytes(t *testing.T) {
	tests := map[int64]string{
		12:        "12 bytes",
		2048:      "2 KB",
		61115904:  "58 MB",
		1 << 30:   "1.0 GB",
		153 << 20: "153 MB",
	}
	for n, want := range tests {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestProblems(t *testing.T) {
	cause := errors.New("dial tcp: i/o timeout")
	p := PreNet("github.com", cause)
	if p.Code != "E-PRE-NET" || !errors.Is(p, cause) || p.IsWarning() {
		t.Errorf("PreNet = %+v", p)
	}
	if !strings.Contains(p.Lines[0], "github.com") {
		t.Errorf("PreNet must name the host: %q", p.Lines)
	}
	if w := Extra("helm", PreOS("macOS 11", "macOS 13")); w.Code != "W-EXTRA-HELM" || !strings.Contains(w.Lines[0], "newer system") {
		t.Errorf("Extra with E-PRE-OS = %+v", w)
	}
	if w := Extra("helm", PreNet("get.helm.sh", errors.New("timeout"))); w.Code != "W-EXTRA-HELM" || !strings.Contains(w.Lines[0], "could not be installed") {
		t.Errorf("Extra with E-PRE-NET = %+v", w)
	}
	if e := ProdForbidden("ns-x-prod"); e.IsWarning() || e.Code != "E-PROD-FORBIDDEN" || !strings.Contains(e.Lines[0], "ns-x-prod") {
		t.Errorf("ProdForbidden = %+v", e)
	}
	if w := Skew("kubectl", "1.34.1", "1.37.2"); !w.IsWarning() || w.Code != "W-SKEW" || !strings.Contains(w.Lines[0], "kubectl (1.34.1)") {
		t.Errorf("Skew = %+v", w)
	}
	if w := ProdNotYet("ns-x-prod"); !w.IsWarning() || w.Code != "W-PROD-NOT-YET" {
		t.Errorf("ProdNotYet = %+v", w)
	}
	if got := Extra("k9s", nil).Code; got != "W-EXTRA-K9S" {
		t.Errorf("Extra code = %q", got)
	}
	f := AccessForbidden("ns-best-team-staging")
	if !strings.Contains(strings.Join(f.Lines, " "), "ns-best-team-staging") || len(f.Next) != 2 {
		t.Errorf("AccessForbidden = %+v", f)
	}
}
