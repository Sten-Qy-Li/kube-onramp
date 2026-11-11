package report

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	// A fake JWT, split so secret scanners don't flag the test file.
	jwt := "eyJ" + "hbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + "eyJ" + "zdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJl"
	tests := []struct {
		name, home, user, clusterUser string
		in, want                      string
	}{
		{name: "windows home", home: `C:\Users\jdoe`, user: "jdoe",
			in: `copied C:\Users\jdoe\AppData\Local\kube-onramp\bin\kubectl.exe`, want: `copied ~\AppData\Local\kube-onramp\bin\kubectl.exe`},
		{name: "windows home other case and slashes", home: `C:\Users\jdoe`, user: "jdoe",
			in: `c:/users/JDOE/.kube/config`, want: `~/.kube/config`},
		{name: "json escaped home", home: `C:\Users\jdoe`, user: "jdoe",
			in: `"command":"C:\\Users\\jdoe\\AppData"`, want: `"command":"~\\AppData"`},
		{name: "mac home", home: "/Users/jdoe", user: "jdoe",
			in: "zshrc at /Users/jdoe/.zshrc", want: "zshrc at ~/.zshrc"},
		{name: "user outside home", home: `C:\Users\jdoe`, user: "jdoe",
			in: `owner LAPTOP\jdoe started`, want: `owner LAPTOP\<user> started`},
		{name: "short user only in home", home: "/Users/al", user: "al",
			in: "all good at /Users/al/x", want: "all good at ~/x"},
		{name: "user inside a longer word stays", home: "", user: "ann",
			in: "annotation by ann", want: "annotation by <user>"},
		{name: "jwt", in: "token " + jwt + " end", want: "token <token> end"},
		{name: "short jwt fragment", in: "Bearer eyJhbGciOiJ", want: "Bearer <token>"},
		{name: "yaml id-token", in: "    id-token: abc.def.ghi", want: "    id-token: <redacted>"},
		{name: "yaml refresh-token", in: "refresh-token: 0123456789", want: "refresh-token: <redacted>"},
		{name: "json access_token", in: `{"access_token":"secret123","x":1}`, want: `{"access_token":"<redacted>","x":1}`},
		{name: "client secret", in: "client-secret=s3cr3t", want: "client-secret=<redacted>"},
		{name: "password", in: "Password: hunter2", want: "Password: <redacted>"},
		{name: "email", in: "logged in as jane.doe@ut.ee today", want: "logged in as <email> today"},
		{name: "cluster user", clusterUser: "oidc:4f2a-jane", in: `"username":"oidc:4f2a-jane"`, want: `"username":"<cluster-user>"`},
		{name: "cluster user that is an email", clusterUser: "jane@ut.ee", in: "user jane@ut.ee", want: "user <cluster-user>"},
		{name: "nothing to hide", home: "/Users/jdoe", user: "jdoe", in: "Step 4 of 8: Installing tools", want: "Step 4 of 8: Installing tools"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRedactor(tt.home, tt.user)
			r.AddClusterUser(tt.clusterUser)
			if got := r.Redact(tt.in); got != tt.want {
				t.Errorf("Redact(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRedactKubeconfigSnippet(t *testing.T) {
	r := NewRedactor("/Users/jdoe", "jdoe")
	in := `users:
- name: etais-user
  user:
    auth-provider:
      config:
        id-token: eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ4In0.sig
        refresh-token: eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjF9.sig2
        client-secret: abc
    exec:
      command: /Users/jdoe/.kube-onramp/bin/kubectl-oidc_login`
	got := r.Redact(in)
	for _, leak := range []string{"eyJ", "abc\n", "jdoe", "sig2"} {
		if strings.Contains(got, leak) {
			t.Errorf("redacted text still contains %q:\n%s", leak, got)
		}
	}
}
