// Package report writes the run log, removes personal data from it, and
// builds the short error report that users paste into Slack or a chatbot.
package report

import (
	"regexp"
	"strings"
	"sync"
)

var (
	// Values of secret-looking keys, in YAML, JSON or key=value form.
	secretValueRe = regexp.MustCompile(`(?i)((?:id|refresh|access)[-_]token|client[-_]secret|password)("?\s*[:=]\s*"?)([^\s"',}]+)`)
	// JWTs and JWT-like fragments.
	jwtRe   = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}(?:\.[A-Za-z0-9_-]*){0,2}`)
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
)

// Redactor removes personal data from text. It is safe for concurrent use.
type Redactor struct {
	mu           sync.Mutex
	homeRes      []*regexp.Regexp
	userRe       *regexp.Regexp
	clusterUsers []*regexp.Regexp
}

// NewRedactor replaces home with "~" and the OS user name with "<user>".
// Either may be empty.
func NewRedactor(home, user string) *Redactor {
	r := &Redactor{}
	home = strings.TrimRight(home, `/\`)
	if len(home) > 1 {
		variants := []string{home, strings.ReplaceAll(home, `\`, `/`), strings.ReplaceAll(home, `\`, `\\`)}
		seen := map[string]bool{}
		for _, v := range variants {
			if seen[v] {
				continue
			}
			seen[v] = true
			r.homeRes = append(r.homeRes, regexp.MustCompile(`(?i)`+regexp.QuoteMeta(v)))
		}
	}
	// Short names would also hide ordinary words, so they are only removed
	// as part of the home directory.
	if len(user) >= 3 {
		r.userRe = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(user) + `\b`)
	}
	return r
}

// AddClusterUser hides a cluster user name (from `kubectl auth whoami`).
// It is shown on screen, but never kept in logs or reports.
func (r *Redactor) AddClusterUser(name string) {
	if len(name) < 3 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusterUsers = append(r.clusterUsers, regexp.MustCompile(`(?i)`+regexp.QuoteMeta(name)))
}

// Redact returns s without secrets and personal data.
func (r *Redactor) Redact(s string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s = secretValueRe.ReplaceAllString(s, "${1}${2}<redacted>")
	s = jwtRe.ReplaceAllString(s, "<token>")
	for _, re := range r.clusterUsers {
		s = re.ReplaceAllString(s, "<cluster-user>")
	}
	s = emailRe.ReplaceAllString(s, "<email>")
	for _, re := range r.homeRes {
		s = re.ReplaceAllString(s, "~")
	}
	if r.userRe != nil {
		s = r.userRe.ReplaceAllString(s, "<user>")
	}
	return s
}
