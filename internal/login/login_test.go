package login

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		timedOut bool
		want     Kind
	}{
		{"our timeout", "", true, TimedOut},
		{"kubelogin timeout", "error: get-token: authentication error: authcode-browser error: authorization code flow error: context deadline exceeded", false, TimedOut},
		{"unauthorized", "error: You must be logged in to the server (Unauthorized)", false, Rejected},
		{"credentials", "the server has asked for the client to provide credentials", false, Rejected},
		{"invalid grant", `oauth2: "invalid_grant" "Token is not active"`, false, Rejected},
		{"port in use (unix)", "error: could not start a local server: listen tcp 127.0.0.1:18000: bind: address already in use", false, Ports},
		{"port in use (windows)", "listen tcp 127.0.0.1:8000: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.", false, Ports},
		{"cluster down", "Unable to connect to the server: dial tcp 193.40.0.1:6443: i/o timeout", true, Network},
		{"dns", "oidc discovery error: Get \"https://keycloak.hpc.ut.ee/realms/ETAIS/.well-known/openid-configuration\": dial tcp: lookup keycloak.hpc.ut.ee: no such host", false, Network},
		{"other", "error: unknown flag: --oidc-foo", false, Other},
	}
	for _, tt := range tests {
		if got := Classify(tt.out, tt.timedOut); got != tt.want {
			t.Errorf("%s: Classify = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestFindURL(t *testing.T) {
	line := "Please visit the following URL in your browser manually: http://localhost:8000"
	if u, ok := FindURL(line); !ok || u != "http://localhost:8000" {
		t.Errorf("FindURL = %q, %v", u, ok)
	}
	if u, ok := FindURL("Please visit the following URL in your browser: http://localhost:18000"); !ok || u != "http://localhost:18000" {
		t.Errorf("FindURL (skip-open-browser form) = %q, %v", u, ok)
	}
	if _, ok := FindURL("error: could not open the browser: exec: \"xdg-open\": not found"); ok {
		t.Error("found a URL in a line without one")
	}
}

func TestParseUsername(t *testing.T) {
	data := []byte(`{"kind":"SelfSubjectReview","apiVersion":"authentication.k8s.io/v1","metadata":{},"status":{"userInfo":{"username":"oidc:jane","groups":["system:authenticated"]}}}`)
	if got, err := ParseUsername(data); err != nil || got != "oidc:jane" {
		t.Errorf("ParseUsername = %q, %v", got, err)
	}
	if _, err := ParseUsername([]byte(`{"status":{}}`)); err == nil {
		t.Error("want error for missing user name")
	}
}

func TestClearTokenCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "oidc-login")
	if err := ClearTokenCache(dir); err != nil {
		t.Errorf("missing dir: %v", err)
	}
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "abc123"), []byte("{}"), 0o600)
	if err := ClearTokenCache(dir); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("cache not empty: %v", entries)
	}
}
