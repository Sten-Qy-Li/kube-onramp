package report

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
)

// Info describes the machine and the tool versions for the report header.
type Info struct {
	Version   string // kube-onramp version
	OS        string // "Windows 11 Education 25H2 x64 (build 26300)"
	Shell     string // "PowerShell 5.1", "zsh"
	Kubectl   string // "v1.36.6", or "" if unknown
	Kubelogin string
}

// ReportLines is how many log lines the report includes.
const ReportLines = 30

// Build creates the short error report. The result is redacted again as a
// last safety net.
func Build(p *problem.Problem, info Info, tail []string, red *Redactor) string {
	or := func(s string) string {
		if s == "" {
			return "unknown"
		}
		return s
	}
	var b strings.Builder
	b.WriteString("=== kube-onramp error report (paste this into an AI chatbot) ===\n")
	b.WriteString("kube-onramp installs kubectl and kubelogin (int128) and sets up OIDC access to a Kubernetes cluster.\n")
	b.WriteString("Trying to: " + or(p.Trying) + "\n")
	b.WriteString("Error code: " + p.Code + "\n")
	if len(p.Lines) > 0 {
		b.WriteString("Message: " + strings.Join(trimAll(p.Lines), " ") + "\n")
	}
	b.WriteString("Tool: kube-onramp " + or(info.Version) + " | OS: " + or(info.OS) + " | " + or(info.Shell) +
		" | kubectl " + or(info.Kubectl) + " | kubelogin " + or(info.Kubelogin) + "\n")
	b.WriteString("--- last 30 log lines (personal data removed) ---\n")
	for _, l := range tail {
		b.WriteString(l + "\n")
	}
	b.WriteString("=== end of report ===\n")
	out := b.String()
	if red != nil {
		out = red.Redact(out)
	}
	return out
}

// Save writes the report to logs/report-<timestamp>.txt and keeps the last
// reports only.
func Save(dir string, now time.Time, text string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "report-"+now.Format("20060102-150405")+".txt")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", err
	}
	Prune(dir, ".txt", KeepLogs)
	return path, nil
}

func trimAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSpace(l)
	}
	return out
}
