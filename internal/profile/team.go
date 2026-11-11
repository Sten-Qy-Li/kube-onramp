package profile

import (
	"regexp"
	"strings"
)

// maxNamespaceLen is the DNS-1123 label limit that Kubernetes uses for namespaces.
const maxNamespaceLen = 63

var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// TeamError explains why a team name or namespace can't be used.
// Its message is shown to users, so it is plain B1 English.
type TeamError struct {
	Input  string
	Reason string
}

func (e *TeamError) Error() string { return e.Reason }

// NormalizeTeam applies the course rule: lowercase, and spaces and
// underscores become hyphens ("Best_Team" -> "best-team"). A full namespace
// typed by mistake ("ns-best-team-staging") is reduced to the team name.
// The result must be a DNS-1123 label, and the namespaces built from it must
// fit in 63 characters.
func (p *Profile) NormalizeTeam(input string) (string, error) {
	team := strings.ToLower(strings.TrimSpace(input))
	team = strings.NewReplacer(" ", "-", "_", "-").Replace(team)
	if team == "" {
		return "", &TeamError{Input: input, Reason: "The team name is empty."}
	}
	for _, tmpl := range []string{p.Namespaces.Staging, p.Namespaces.Prod} {
		if inner, ok := matchTemplate(tmpl, team); ok {
			team = inner
			break
		}
	}
	if err := checkLabel(team, input, "team name"); err != nil {
		return "", err
	}
	for _, tmpl := range []string{p.Namespaces.Staging, p.Namespaces.Prod} {
		if ns := Namespace(tmpl, team); len(ns) > maxNamespaceLen {
			return "", &TeamError{Input: input, Reason: "The team name is too long. The namespace " + ns + " has more than 63 characters."}
		}
	}
	return team, nil
}

// StagingNamespace returns the staging namespace for a normalized team name.
func (p *Profile) StagingNamespace(team string) string { return Namespace(p.Namespaces.Staging, team) }

// ProdNamespace returns the prod namespace for a normalized team name.
func (p *Profile) ProdNamespace(team string) string { return Namespace(p.Namespaces.Prod, team) }

// Namespace fills a namespace template.
func Namespace(tmpl, team string) string { return strings.Replace(tmpl, TeamPlaceholder, team, 1) }

// ValidateNamespace checks a namespace given directly (--staging-namespace,
// --prod-namespace).
func ValidateNamespace(ns string) error {
	if len(ns) > maxNamespaceLen {
		return &TeamError{Input: ns, Reason: "The namespace " + ns + " is too long. It can have at most 63 characters."}
	}
	return checkLabel(ns, ns, "namespace")
}

// matchTemplate returns the {team} part if s has the template's form.
func matchTemplate(tmpl, s string) (string, bool) {
	prefix, suffix, ok := strings.Cut(tmpl, TeamPlaceholder)
	if !ok || len(s) <= len(prefix)+len(suffix) {
		return "", false
	}
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, suffix) {
		return "", false
	}
	return s[len(prefix) : len(s)-len(suffix)], true
}

func checkLabel(s, input, what string) error {
	if s == "" {
		return &TeamError{Input: input, Reason: "The " + what + " is empty."}
	}
	if dns1123Label.MatchString(s) {
		return nil
	}
	if strings.Trim(s, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return &TeamError{Input: input, Reason: "A " + what + " can only have the letters a-z, the numbers 0-9 and hyphens (-)."}
	}
	return &TeamError{Input: input, Reason: "A " + what + " must start and end with a letter or a number."}
}
