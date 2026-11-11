// Package catalog loads and validates catalog/tools.yaml: the tools
// kube-onramp installs, with pinned versions and checksums per platform.
package catalog

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	kubeonramp "github.com/Sten-Qy-Li/kube-onramp"
)

// SchemaVersion is the catalog format this build understands.
const SchemaVersion = 1

// RequiredPlatforms must be present for every tool (GOOS/GOARCH).
var RequiredPlatforms = []string{"windows/amd64", "darwin/amd64", "darwin/arm64", "linux/amd64"}

var knownPlatforms = map[string]bool{
	"windows/amd64": true, "windows/arm64": true,
	"darwin/amd64": true, "darwin/arm64": true,
	"linux/amd64": true, "linux/arm64": true,
}

// Role says whether a tool is needed for cluster access or optional.
type Role string

// Roles.
const (
	RoleCore  Role = "core"
	RoleExtra Role = "extra"
)

// Archive is how a download is packed.
type Archive string

// Archive types.
const (
	ArchiveNone  Archive = "none" // the download is the binary itself
	ArchiveZip   Archive = "zip"
	ArchiveTarGz Archive = "tar.gz"
)

// Catalog is the parsed catalog file.
type Catalog struct {
	SchemaVersion int    `json:"schemaVersion"`
	Tools         []Tool `json:"tools"`
}

// Tool is one installable program.
type Tool struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    Role   `json:"role"`
	Version string `json:"version"`
	// Binary is the installed file name without ".exe".
	Binary         string              `json:"binary"`
	VersionCommand []string            `json:"versionCommand"`
	VersionRegex   string              `json:"versionRegex"`
	Identity       Identity            `json:"identity"`
	Platforms      map[string]Artifact `json:"platforms"`
}

// Identity tells the right product apart from unrelated programs with the
// same name (int128 kubelogin vs. Azure kubelogin).
type Identity struct {
	Command      []string `json:"command"`
	MustMatch    string   `json:"mustMatch"`
	MustNotMatch string   `json:"mustNotMatch,omitempty"`
}

// Artifact is the download for one platform.
type Artifact struct {
	URL     string  `json:"url"`
	SHA256  string  `json:"sha256"`
	Size    int64   `json:"size"`
	Archive Archive `json:"archive"`
	// Path is the binary's path inside the archive (empty for ArchiveNone).
	Path string `json:"path,omitempty"`
	// MinOS is compared with the OS version: Windows NT version (10.0),
	// macOS product version (12.0) or Linux kernel version (3.2).
	MinOS string `json:"minOS"`
}

var (
	idPattern      = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	dottedPattern  = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
)

// Load parses the catalog built into the binary.
func Load() (*Catalog, error) {
	return Parse(kubeonramp.Catalog)
}

// Parse decodes a catalog strictly (unknown keys are errors) and validates it.
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	if err := yaml.UnmarshalStrict(data, &c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate reports every problem in the catalog, not just the first.
func (c *Catalog) Validate() error {
	var errs []error
	if c.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Errorf("schemaVersion is %d, want %d", c.SchemaVersion, SchemaVersion))
	}
	if len(c.Tools) == 0 {
		errs = append(errs, errors.New("no tools"))
	}
	seen := map[string]bool{}
	for i := range c.Tools {
		t := &c.Tools[i]
		if seen[t.ID] {
			errs = append(errs, fmt.Errorf("tool %q is listed twice", t.ID))
		}
		seen[t.ID] = true
		if err := t.validate(); err != nil {
			errs = append(errs, fmt.Errorf("tool %q: %w", t.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (t *Tool) validate() error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if !idPattern.MatchString(t.ID) {
		bad("id is not valid")
	}
	if t.Name == "" {
		bad("name is empty")
	}
	if t.Role != RoleCore && t.Role != RoleExtra {
		bad("role %q must be %q or %q", t.Role, RoleCore, RoleExtra)
	}
	if !versionPattern.MatchString(t.Version) {
		bad("version %q must look like 1.2.3", t.Version)
	}
	if t.Binary == "" || strings.ContainsAny(t.Binary, `/\`) || strings.HasSuffix(strings.ToLower(t.Binary), ".exe") {
		bad("binary %q must be a plain file name without .exe", t.Binary)
	}
	if len(t.VersionCommand) == 0 {
		bad("versionCommand is empty")
	}
	if re, err := regexp.Compile(t.VersionRegex); err != nil || t.VersionRegex == "" {
		bad("versionRegex does not compile: %v", err)
	} else if re.NumSubexp() != 1 {
		bad("versionRegex must have exactly one capture group")
	}
	if len(t.Identity.Command) == 0 {
		bad("identity.command is empty")
	}
	if _, err := regexp.Compile(t.Identity.MustMatch); err != nil || t.Identity.MustMatch == "" {
		bad("identity.mustMatch is empty or does not compile")
	}
	if _, err := regexp.Compile(t.Identity.MustNotMatch); err != nil {
		bad("identity.mustNotMatch does not compile: %v", err)
	}
	for _, p := range RequiredPlatforms {
		if _, ok := t.Platforms[p]; !ok {
			bad("platform %s is missing", p)
		}
	}
	for p, a := range t.Platforms {
		if !knownPlatforms[p] {
			bad("platform %q is not known", p)
			continue
		}
		if err := a.validate(t.Version); err != nil {
			bad("%s: %v", p, err)
		}
	}
	return errors.Join(errs...)
}

func (a *Artifact) validate(version string) error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if u, err := url.Parse(a.URL); err != nil || u.Scheme != "https" || u.Host == "" {
		bad("url %q must be an https URL", a.URL)
	} else if !strings.Contains(a.URL, version) {
		// Catches a version bump that forgot to update the URL.
		bad("url %q does not contain the version %s", a.URL, version)
	}
	if !sha256Pattern.MatchString(a.SHA256) {
		bad("sha256 must be 64 lowercase hex characters")
	}
	if a.Size <= 0 {
		bad("size must be the download size in bytes")
	}
	switch a.Archive {
	case ArchiveNone:
		if a.Path != "" {
			bad("path must be empty when archive is none")
		}
	case ArchiveZip, ArchiveTarGz:
		if a.Path == "" || path.IsAbs(a.Path) || path.Clean(a.Path) != a.Path || strings.HasPrefix(a.Path, "..") {
			bad("path %q must be a clean relative path inside the archive", a.Path)
		}
	default:
		bad("archive %q must be none, zip or tar.gz", a.Archive)
	}
	if !dottedPattern.MatchString(a.MinOS) {
		bad("minOS %q must look like 10.0 or 12.0", a.MinOS)
	}
	return errors.Join(errs...)
}

// Tool returns the tool with the given id.
func (c *Catalog) Tool(id string) (*Tool, bool) {
	for i := range c.Tools {
		if c.Tools[i].ID == id {
			return &c.Tools[i], true
		}
	}
	return nil, false
}

// ByRole returns the tools with the given role, in catalog order.
func (c *Catalog) ByRole(r Role) []*Tool {
	var out []*Tool
	for i := range c.Tools {
		if c.Tools[i].Role == r {
			out = append(out, &c.Tools[i])
		}
	}
	return out
}

// Artifact returns the download for a platform.
func (t *Tool) Artifact(goos, goarch string) (Artifact, bool) {
	a, ok := t.Platforms[goos+"/"+goarch]
	return a, ok
}

// FileName is the installed file name on goos.
func (t *Tool) FileName(goos string) string {
	if goos == "windows" {
		return t.Binary + ".exe"
	}
	return t.Binary
}

// ParseVersion extracts the version from the output of VersionCommand.
// ok is false when the output has no version (some builds print none).
func (t *Tool) ParseVersion(output string) (version string, ok bool) {
	m := regexp.MustCompile(t.VersionRegex).FindStringSubmatch(output)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// IsRightProduct reports whether the output of Identity.Command belongs to
// this tool.
func (t *Tool) IsRightProduct(output string) bool {
	if !regexp.MustCompile(t.Identity.MustMatch).MatchString(output) {
		return false
	}
	if t.Identity.MustNotMatch != "" && regexp.MustCompile(t.Identity.MustNotMatch).MatchString(output) {
		return false
	}
	return true
}

// SupportsOS reports whether an OS version meets the artifact's minimum.
func (a Artifact) SupportsOS(osVersion string) (bool, error) {
	c, err := CompareVersions(osVersion, a.MinOS)
	if err != nil {
		return false, err
	}
	return c >= 0, nil
}

// CompareVersions compares dotted numeric versions ("13.7.8", "10.0.19045").
// A leading "v" is ignored and missing parts count as 0, so "12" == "12.0".
func CompareVersions(a, b string) (int, error) {
	pa, err := splitVersion(a)
	if err != nil {
		return 0, err
	}
	pb, err := splitVersion(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func splitVersion(v string) ([]int, error) {
	v = strings.TrimPrefix(v, "v")
	if !dottedPattern.MatchString(v) {
		return nil, fmt.Errorf("%q is not a version number", v)
	}
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("%q is not a version number", v)
		}
		out[i] = n
	}
	return out, nil
}
