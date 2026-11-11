package pathenv

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRCFile(t *testing.T) {
	home := filepath.FromSlash("/Users/test")
	tests := []struct{ shell, zdotdir, goos, want string }{
		{"/bin/zsh", "", "darwin", filepath.Join(home, ".zshrc")},
		{"/bin/zsh", filepath.FromSlash("/tmp/z"), "darwin", filepath.Join(filepath.FromSlash("/tmp/z"), ".zshrc")},
		{"/bin/bash", "", "darwin", filepath.Join(home, ".bash_profile")},
		{"/usr/bin/bash", "", "linux", filepath.Join(home, ".bashrc")},
		{"/usr/local/bin/fish", "", "darwin", filepath.Join(home, ".profile")},
	}
	for _, tt := range tests {
		if got := RCFile(tt.shell, home, tt.zdotdir, tt.goos); got != tt.want {
			t.Errorf("RCFile(%q, zdotdir %q) = %q, want %q", tt.shell, tt.zdotdir, got, tt.want)
		}
	}
}

func TestParseShellOutput(t *testing.T) {
	names := []string{"kubectl", "kubectl-oidc_login", "kube-onramp"}
	out := "Welcome! Last login: today\nPATH=noise\n" +
		"__KUBE_ONRAMP_BEGIN__\n" +
		"PATH=/Users/test/.kube-onramp/bin:/usr/local/bin:/usr/bin\n" +
		"kubectl=/Users/test/.kube-onramp/bin/kubectl\n" +
		"kubectl-oidc_login=/usr/local/bin/kubectl-oidc_login\n" +
		"kube-onramp=\n" +
		"__KUBE_ONRAMP_END__\n" +
		"logout\n"
	path, found, ok := ParseShellOutput(out, names)
	if !ok {
		t.Fatal("not ok")
	}
	if path != "/Users/test/.kube-onramp/bin:/usr/local/bin:/usr/bin" {
		t.Errorf("path = %q", path)
	}
	want := map[string]string{
		"kubectl":            "/Users/test/.kube-onramp/bin/kubectl",
		"kubectl-oidc_login": "/usr/local/bin/kubectl-oidc_login",
	}
	if !reflect.DeepEqual(found, want) {
		t.Errorf("found = %v, want %v", found, want)
	}
	if _, _, ok := ParseShellOutput("zsh: command not found\n", names); ok {
		t.Error("want not ok without markers")
	}
}
