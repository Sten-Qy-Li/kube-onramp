package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Sten-Qy-Li/kube-onramp/internal/app"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOpts *app.Options // nil: app.Run must not be called
		wantOut  string
	}{
		{name: "version", args: []string{"version"}, wantOut: "kube-onramp dev"},
		{name: "help flag", args: []string{"--help"}, wantOut: "Usage:"},
		{name: "default is setup", args: []string{"--team", "Best_Team", "--yes"},
			wantOpts: &app.Options{Command: "setup", Team: "Best_Team", Yes: true}},
		{name: "command first", args: []string{"check", "--skip-login"},
			wantOpts: &app.Options{Command: "check", SkipLogin: true}},
		{name: "flags before and after the command", args: []string{"--minimal", "setup", "-yes", "--team=x"},
			wantOpts: &app.Options{Command: "setup", Minimal: true, Yes: true, Team: "x"}},
		{name: "reset", args: []string{"reset", "--yes"}, wantOpts: &app.Options{Command: "reset", Yes: true}},
		{name: "unknown flag", args: []string{"--tema", "x"}, wantCode: app.ExitUsage},
		{name: "unknown command", args: []string{"install"}, wantCode: app.ExitUsage},
		{name: "two commands", args: []string{"check", "reset"}, wantCode: app.ExitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *app.Options
			fake := func(_ context.Context, o app.Options, _ string) int {
				got = &o
				return 0
			}
			var out, errOut bytes.Buffer
			code := run(context.Background(), tt.args, &out, &errOut, fake)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, errOut.String())
			}
			if (got == nil) != (tt.wantOpts == nil) || (got != nil && *got != *tt.wantOpts) {
				t.Errorf("options = %+v, want %+v", got, tt.wantOpts)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), tt.wantOut)
			}
		})
	}
}
