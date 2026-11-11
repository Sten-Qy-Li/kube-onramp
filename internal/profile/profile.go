// Package profile loads cluster profiles (profiles/*.yaml) and turns a team
// name into the namespaces the profile uses.
package profile

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"

	kubeonramp "github.com/Sten-Qy-Li/kube-onramp"
)

// DefaultID is the profile used when --profile is not given.
const DefaultID = "ut-devops"

// TeamPlaceholder is replaced by the normalized team name in namespace templates.
const TeamPlaceholder = "{team}"

// Profile describes one cluster: how to reach it, how to log in, and which
// kubeconfig entries and namespaces belong to it.
type Profile struct {
	ID           string          `json:"id"`
	DisplayName  string          `json:"displayName"`
	ClusterNoun  string          `json:"clusterNoun"`
	Cluster      Cluster         `json:"cluster"`
	OIDC         OIDC            `json:"oidc"`
	Exec         Exec            `json:"exec"`
	Kubeconfig   KubeconfigNames `json:"kubeconfig"`
	Namespaces   Namespaces      `json:"namespaces"`
	TeamExample  string          `json:"teamExample"`
	AccessChecks AccessChecks    `json:"accessChecks"`
	LoginWording string          `json:"loginWording"`
	HelpLinks    []Link          `json:"helpLinks"`
}

// Cluster is the API server and its CA, as written into the kubeconfig.
type Cluster struct {
	Server string `json:"server"`
	// CertificateAuthorityData is base64-encoded PEM, like the kubeconfig field.
	CertificateAuthorityData string `json:"certificateAuthorityData"`
}

// OIDC holds the kubelogin settings.
type OIDC struct {
	IssuerURL string   `json:"issuerURL"`
	ClientID  string   `json:"clientID"`
	ExtraArgs []string `json:"extraArgs"`
}

// Exec holds settings for the kubeconfig exec entry.
type Exec struct {
	APIVersion string `json:"apiVersion"`
}

// KubeconfigNames are the entry names kube-onramp owns in the kubeconfig.
type KubeconfigNames struct {
	Cluster  string   `json:"cluster"`
	User     string   `json:"user"`
	Contexts Contexts `json:"contexts"`
}

// Contexts are the kubeconfig context names.
type Contexts struct {
	Staging string `json:"staging"`
	Prod    string `json:"prod"`
}

// Namespaces are templates that contain TeamPlaceholder.
type Namespaces struct {
	Staging string `json:"staging"`
	Prod    string `json:"prod"`
}

// AccessChecks are the `kubectl auth can-i` checks per namespace.
type AccessChecks struct {
	Staging CheckSet `json:"staging"`
	Prod    CheckSet `json:"prod"`
}

// CheckSet splits checks into ones that must pass and ones only reported.
type CheckSet struct {
	Required      []Check `json:"required"`
	Informational []Check `json:"informational"`
}

// Check is one `kubectl auth can-i <verb> <resource>`.
type Check struct {
	Verb     string `json:"verb"`
	Resource string `json:"resource"`
}

// Link is a help link shown in the summary.
type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ErrUnknown means there is no profile with the requested id.
var ErrUnknown = errors.New("unknown profile")

// LoadEmbedded loads a profile that is built into the binary.
func LoadEmbedded(id string) (*Profile, error) {
	return Load(kubeonramp.Profiles, id)
}

// Load reads profiles/<id>.yaml from fsys.
func Load(fsys fs.FS, id string) (*Profile, error) {
	if !idPattern.MatchString(id) {
		return nil, fmt.Errorf("profile id %q is not valid: %w", id, ErrUnknown)
	}
	data, err := fs.ReadFile(fsys, "profiles/"+id+".yaml")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("there is no profile called %q: %w", id, ErrUnknown)
	}
	if err != nil {
		return nil, err
	}
	p, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("profile %q: %w", id, err)
	}
	if p.ID != id {
		return nil, fmt.Errorf("profile file %q has id %q", id, p.ID)
	}
	return p, nil
}

// Parse decodes a profile strictly (unknown keys are errors) and validates it.
func Parse(data []byte) (*Profile, error) {
	var p Profile
	if err := yaml.UnmarshalStrict(data, &p); err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate reports every problem in the profile, not just the first.
func (p *Profile) Validate() error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if !idPattern.MatchString(p.ID) {
		bad("id %q is not valid", p.ID)
	}
	if p.DisplayName == "" {
		bad("displayName is empty")
	}
	if p.ClusterNoun == "" {
		bad("clusterNoun is empty")
	}
	if err := checkHTTPS(p.Cluster.Server); err != nil {
		bad("cluster.server: %v", err)
	}
	if _, err := p.CACertificate(); err != nil {
		bad("cluster.certificateAuthorityData: %v", err)
	}
	if err := checkHTTPS(p.OIDC.IssuerURL); err != nil {
		bad("oidc.issuerURL: %v", err)
	}
	if p.OIDC.ClientID == "" {
		bad("oidc.clientID is empty")
	}
	for _, a := range p.OIDC.ExtraArgs {
		if !strings.HasPrefix(a, "--") {
			bad("oidc.extraArgs: %q must start with --", a)
		}
	}
	if p.Exec.APIVersion == "" {
		bad("exec.apiVersion is empty")
	}
	names := map[string]string{
		"kubeconfig.cluster":          p.Kubeconfig.Cluster,
		"kubeconfig.user":             p.Kubeconfig.User,
		"kubeconfig.contexts.staging": p.Kubeconfig.Contexts.Staging,
		"kubeconfig.contexts.prod":    p.Kubeconfig.Contexts.Prod,
	}
	for field, v := range names {
		if v == "" {
			bad("%s is empty", field)
		}
	}
	if p.Kubeconfig.Contexts.Staging != "" && p.Kubeconfig.Contexts.Staging == p.Kubeconfig.Contexts.Prod {
		bad("kubeconfig.contexts: staging and prod have the same name")
	}
	for field, tmpl := range map[string]string{"namespaces.staging": p.Namespaces.Staging, "namespaces.prod": p.Namespaces.Prod} {
		if strings.Count(tmpl, TeamPlaceholder) != 1 {
			bad("%s must contain %s exactly once", field, TeamPlaceholder)
		}
	}
	if _, err := p.NormalizeTeam(p.TeamExample); err != nil {
		bad("teamExample: %v", err)
	}
	if len(p.AccessChecks.Staging.Required) == 0 {
		bad("accessChecks.staging.required is empty")
	}
	for _, set := range []CheckSet{p.AccessChecks.Staging, p.AccessChecks.Prod} {
		for _, c := range append(append([]Check{}, set.Required...), set.Informational...) {
			if c.Verb == "" || c.Resource == "" {
				bad("accessChecks: every check needs a verb and a resource")
			}
		}
	}
	if p.LoginWording == "" {
		bad("loginWording is empty")
	}
	for _, l := range p.HelpLinks {
		if l.Name == "" {
			bad("helpLinks: a link has no name")
		}
		if err := checkHTTPS(l.URL); err != nil {
			bad("helpLinks %q: %v", l.Name, err)
		}
	}
	return errors.Join(errs...)
}

// CACertificate decodes and parses the cluster CA.
func (p *Profile) CACertificate() (*x509.Certificate, error) {
	raw, err := base64.StdEncoding.DecodeString(p.Cluster.CertificateAuthorityData)
	if err != nil {
		return nil, fmt.Errorf("not valid base64: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("not a PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

func checkHTTPS(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%q must be an https URL", s)
	}
	return nil
}
