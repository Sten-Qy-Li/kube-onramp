package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "kube-onramp dev") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunScaffoldRefusesSetup(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if strings.Contains(errOut.String(), "E-INTERNAL") {
		t.Errorf("embedded data is broken: %s", errOut.String())
	}
}
