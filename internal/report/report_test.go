package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
)

func TestBuildIsRedactedAndShaped(t *testing.T) {
	red := NewRedactor(`C:\Users\jdoe`, "jdoe")
	red.AddClusterUser("jane@ut.ee")
	log := NewMemoryLogger(red)
	log.Printf(`running C:\Users\jdoe\AppData\Local\kube-onramp\bin\kubectl.exe auth whoami`)
	log.Printf(`{"status":{"userInfo":{"username":"jane@ut.ee"}}}`)
	log.Printf("id-token: eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ4In0.sig")

	p := problem.LoginTimeout()
	p.Trying = "log in to the cluster through the browser (step 6 of 8)"
	text := Build(p, Info{Version: "v0.1.0", OS: "Windows 10 Pro 22H2 x64 (build 19045)", Shell: "PowerShell 5.1", Kubectl: "v1.36.6"}, log.Tail(ReportLines), red)

	for _, want := range []string{
		"=== kube-onramp error report (paste this into an AI chatbot) ===\n",
		"Trying to: log in to the cluster through the browser (step 6 of 8)\n",
		"Error code: E-LOGIN-TIMEOUT\n",
		"Tool: kube-onramp v0.1.0 | OS: Windows 10 Pro 22H2 x64 (build 19045) | PowerShell 5.1 | kubectl v1.36.6 | kubelogin unknown\n",
		"--- last 30 log lines (personal data removed) ---\n",
		`running ~\AppData\Local\kube-onramp\bin\kubectl.exe auth whoami`,
		`"username":"<cluster-user>"`,
		"=== end of report ===\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report misses %q:\n%s", want, text)
		}
	}
	for _, leak := range []string{"jdoe", "jane", "eyJ"} {
		if strings.Contains(text, leak) {
			t.Errorf("report leaks %q:\n%s", leak, text)
		}
	}
}

func TestLogFileAndPrune(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < KeepLogs+3; i++ {
		name := filepath.Join(dir, "2026010"+string(rune('0'+i%10))+"-00000"+string(rune('0'+i/10))+".log")
		if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	red := NewRedactor("/Users/jdoe", "jdoe")
	l, err := OpenLog(dir, time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC), red)
	if err != nil {
		t.Fatal(err)
	}
	l.Printf("home is /Users/jdoe\nsecond line")
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "jdoe") || !strings.Contains(string(data), "home is ~") || !strings.Contains(string(data), "second line") {
		t.Errorf("log content = %q", data)
	}
	logs, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	if len(logs) != KeepLogs {
		t.Errorf("kept %d logs, want %d", len(logs), KeepLogs)
	}
	if got := l.Tail(1); len(got) != 1 || got[0] != "second line" {
		t.Errorf("Tail = %q", got)
	}
}
