package profile

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedProfile(t *testing.T) {
	p, err := LoadEmbedded(DefaultID)
	if err != nil {
		t.Fatalf("LoadEmbedded(%q): %v", DefaultID, err)
	}
	cert, err := p.CACertificate()
	if err != nil {
		t.Fatalf("CACertificate: %v", err)
	}
	if cert.Subject.CommonName != "kubernetes" {
		t.Errorf("CA common name = %q, want kubernetes", cert.Subject.CommonName)
	}
	if got := p.StagingNamespace("best-team"); got != "ns-best-team-staging" {
		t.Errorf("StagingNamespace = %q", got)
	}
	if got := p.ProdNamespace("best-team"); got != "ns-best-team-prod" {
		t.Errorf("ProdNamespace = %q", got)
	}
}

func TestLoadErrors(t *testing.T) {
	fsys := fstest.MapFS{
		"profiles/other.yaml": {Data: []byte("id: something-else\n")},
	}
	for _, id := range []string{"", "../etc/passwd", "UT", "missing", "other"} {
		if _, err := Load(fsys, id); err == nil {
			t.Errorf("Load(%q): want error", id)
		}
	}
}

func TestNormalizeTeam(t *testing.T) {
	p := mustDefault(t)
	tests := []struct {
		in      string
		want    string
		wantErr string
	}{
		{in: "best-team", want: "best-team"},
		{in: "Best_Team", want: "best-team"},
		{in: "best team", want: "best-team"},
		{in: "  Best-Team \t", want: "best-team"},
		{in: "team7", want: "team7"},
		{in: "7", want: "7"},
		{in: "ns-best-team-staging", want: "best-team"},
		{in: "NS-Best_Team-Staging", want: "best-team"},
		{in: "ns-best-team-prod", want: "best-team"},
		{in: "ns-x-staging", want: "x"},
		{in: strings.Repeat("a", 52), want: strings.Repeat("a", 52)},
		{in: "", wantErr: "empty"},
		{in: "   ", wantErr: "empty"},
		{in: "-team", wantErr: "start and end"},
		{in: "team-", wantErr: "start and end"},
		{in: "_team", wantErr: "start and end"},
		{in: "team.name", wantErr: "only have"},
		{in: "tëam", wantErr: "only have"},
		{in: "team/../x", wantErr: "only have"},
		{in: strings.Repeat("a", 53), wantErr: "too long"},
	}
	for _, tt := range tests {
		got, err := p.NormalizeTeam(tt.in)
		if tt.wantErr != "" {
			var te *TeamError
			if !errors.As(err, &te) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("NormalizeTeam(%q) error = %v, want TeamError containing %q", tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("NormalizeTeam(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestValidateNamespace(t *testing.T) {
	for _, ns := range []string{"ns-best-team-staging", "a", strings.Repeat("a", 63)} {
		if err := ValidateNamespace(ns); err != nil {
			t.Errorf("ValidateNamespace(%q) = %v", ns, err)
		}
	}
	for _, ns := range []string{"", "Ns-x", "ns_x", "-ns", strings.Repeat("a", 64)} {
		if err := ValidateNamespace(ns); err == nil {
			t.Errorf("ValidateNamespace(%q): want error", ns)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Profile)
		wantErr string
	}{
		{"http server", func(p *Profile) { p.Cluster.Server = "http://example.com:6443" }, "cluster.server"},
		{"bad CA base64", func(p *Profile) { p.Cluster.CertificateAuthorityData = "%%%" }, "base64"},
		{"CA not PEM", func(p *Profile) { p.Cluster.CertificateAuthorityData = "aGVsbG8=" }, "PEM"},
		{"no client id", func(p *Profile) { p.OIDC.ClientID = "" }, "clientID"},
		{"extra arg without dashes", func(p *Profile) { p.OIDC.ExtraArgs = []string{"pkce"} }, "extraArgs"},
		{"template without team", func(p *Profile) { p.Namespaces.Staging = "ns-staging" }, "namespaces.staging"},
		{"same contexts", func(p *Profile) { p.Kubeconfig.Contexts.Prod = p.Kubeconfig.Contexts.Staging }, "same name"},
		{"no required checks", func(p *Profile) { p.AccessChecks.Staging.Required = nil }, "required is empty"},
		{"bad team example", func(p *Profile) { p.TeamExample = "Not OK!" }, "teamExample"},
		{"http help link", func(p *Profile) { p.HelpLinks = []Link{{Name: "x", URL: "http://x"}} }, "helpLinks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustDefault(t)
			tt.mutate(p)
			err := p.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRejectsUnknownKeys(t *testing.T) {
	if _, err := Parse([]byte("id: x\nclusterr: {}\n")); err == nil {
		t.Error("want error for unknown key")
	}
}

func mustDefault(t *testing.T) *Profile {
	t.Helper()
	p, err := LoadEmbedded(DefaultID)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
