package catalog

import (
	"strings"
	"testing"
)

func TestEmbeddedCatalogIsValid(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, id := range []string{"kubectl", "kubelogin"} {
		tool, ok := c.Tool(id)
		if !ok {
			t.Fatalf("tool %q missing", id)
		}
		if tool.Role != RoleCore {
			t.Errorf("%s role = %s, want core", id, tool.Role)
		}
	}
	if got := len(c.ByRole(RoleCore)); got != 2 {
		t.Errorf("core tools = %d, want 2", got)
	}
	var extras []string
	for _, tool := range c.ByRole(RoleExtra) {
		extras = append(extras, tool.ID)
	}
	if got, want := strings.Join(extras, ","), "helm,kustomize,kubeconform,trivy,k9s,oha"; got != want {
		t.Errorf("extras = %s, want %s", got, want)
	}
	// Machine B runs macOS 13, the oldest supported macOS (SPEC §2).
	for _, tool := range c.Tools {
		for _, p := range []string{"darwin/amd64", "darwin/arm64"} {
			a := tool.Platforms[p]
			if ok, err := a.SupportsOS("13.0"); err != nil || !ok {
				t.Errorf("%s %s needs macOS %s, newer than 13.0", tool.ID, p, a.MinOS)
			}
		}
	}
	// Only oha's Windows file needs a part that Windows doesn't always have:
	// the Visual C++ runtime (D49, D50). The Go tools need nothing.
	for _, tool := range c.Tools {
		for p, a := range tool.Platforms {
			switch {
			case tool.ID == "oha" && p == "windows/amd64":
				if a.Needs == nil || a.Needs.File != "vcruntime140.dll" || a.Needs.URL != "https://aka.ms/vc14/vc_redist.x64.exe" {
					t.Errorf("oha %s needs = %+v, want vcruntime140.dll", p, a.Needs)
				}
			case a.Needs != nil:
				t.Errorf("%s %s needs %+v, want nothing", tool.ID, p, a.Needs)
			}
		}
	}
	kl, _ := c.Tool("kubelogin")
	if got := kl.FileName("windows"); got != "kubectl-oidc_login.exe" {
		t.Errorf("FileName(windows) = %q", got)
	}
	if got := kl.FileName("darwin"); got != "kubectl-oidc_login" {
		t.Errorf("FileName(darwin) = %q", got)
	}
}

const validTool = `
schemaVersion: 1
tools:
  - id: demo
    name: Demo
    role: core
    version: 1.2.3
    binary: demo
    versionCommand: [version]
    versionRegex: 'demo v([0-9.]+)'
    identity: {command: [--help], mustMatch: 'Demo tool'}
    platforms:
      windows/amd64: {url: "https://example.com/v1.2.3/demo.zip", sha256: "` + hash + `", size: 10, archive: zip, path: demo.exe, minOS: "10.0"}
      darwin/amd64:  {url: "https://example.com/v1.2.3/demo", sha256: "` + hash + `", size: 10, archive: none, minOS: "12.0"}
      darwin/arm64:  {url: "https://example.com/v1.2.3/demo", sha256: "` + hash + `", size: 10, archive: none, minOS: "12.0"}
      linux/amd64:   {url: "https://example.com/v1.2.3/demo.tar.gz", sha256: "` + hash + `", size: 10, archive: tar.gz, path: bin/demo, minOS: "3.2"}
`

const hash = "2bc0fbea066016c62cfef307108c303c2f796420d4a5dfc844de6e25d4bc47e0"

const (
	winMinOS   = `path: demo.exe, minOS: "10.0"`
	validNeeds = `{file: x.dll, package: X Runtime, url: "https://example.com/x"}`
)

func TestValidation(t *testing.T) {
	if _, err := Parse([]byte(validTool)); err != nil {
		t.Fatalf("valid catalog rejected: %v", err)
	}
	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{"schema version", "schemaVersion: 1", "schemaVersion: 2", "schemaVersion"},
		{"unknown key", "    binary: demo", "    binary: demo\n    binaryName: x", "unknown field"},
		{"bad role", "role: core", "role: optional", "role"},
		{"bad version", "version: 1.2.3", "version: latest", "version"},
		{"binary with exe", "binary: demo", "binary: demo.exe", "binary"},
		{"regex without group", "'demo v([0-9.]+)'", "'demo v[0-9.]+'", "capture group"},
		{"empty identity", "mustMatch: 'Demo tool'", "mustMatch: ''", "mustMatch"},
		{"missing platform", "  linux/amd64:", "  linux/arm64:", "linux/amd64 is missing"},
		{"unknown platform", "      linux/amd64:", "      plan9/386: {}\n      linux/amd64:", "not known"},
		{"http url", "https://example.com/v1.2.3/demo.zip", "http://example.com/v1.2.3/demo.zip", "https"},
		{"url without version", "https://example.com/v1.2.3/demo.zip", "https://example.com/latest/demo.zip", "does not contain the version"},
		{"short sha", `sha256: "` + hash + `", size: 10, archive: zip`, `sha256: "abc", size: 10, archive: zip`, "sha256"},
		{"uppercase sha", `sha256: "` + hash + `", size: 10, archive: zip`, `sha256: "` + strings.ToUpper(hash) + `", size: 10, archive: zip`, "sha256"},
		{"no size", "size: 10, archive: zip", "size: 0, archive: zip", "size"},
		{"zip without path", "archive: zip, path: demo.exe,", "archive: zip,", "path"},
		{"path escapes", "path: bin/demo", "path: ../demo", "path"},
		{"path with none", `archive: none, minOS: "12.0"}
      darwin/arm64`, `archive: none, path: x, minOS: "12.0"}
      darwin/arm64`, "path must be empty"},
		{"bad archive", "archive: tar.gz", "archive: rar", "archive"},
		{"bad minOS", `minOS: "3.2"`, `minOS: "Ventura"`, "minOS"},
		{"accept with both rules", "    platforms:", "    accept: {serverMinorSkew: 1, major: 4}\n    platforms:", "not both"},
		{"accept negative skew", "    platforms:", "    accept: {serverMinorSkew: -1}\n    platforms:", "serverMinorSkew"},
		{"accept unknown rule", "    platforms:", "    accept: {minor: 2}\n    platforms:", "unknown field"},
		{"duplicate", "tools:\n", "tools:\n  - {id: demo}\n", "twice"},
		{"needs on macOS", `size: 10, archive: none, minOS: "12.0"}`, `size: 10, archive: none, minOS: "12.0", needs: ` + validNeeds + `}`, "only for Windows"},
		{"needs not a dll", winMinOS, winMinOS + `, needs: {file: x.exe, package: X, url: "https://example.com/x"}`, "needs.file"},
		{"needs with a path", winMinOS, winMinOS + `, needs: {file: 'sub\x.dll', package: X, url: "https://example.com/x"}`, "needs.file"},
		{"needs without package", winMinOS, winMinOS + `, needs: {file: x.dll, package: "", url: "https://example.com/x"}`, "needs.package"},
		{"needs http url", winMinOS, winMinOS + `, needs: {file: x.dll, package: X, url: "http://example.com/x"}`, "needs.url"},
		{"needs unknown key", winMinOS, winMinOS + `, needs: {file: x.dll, package: X, url: "https://example.com/x", dir: C}`, "unknown field"},
	}
	ok := strings.Replace(validTool, winMinOS, winMinOS+", needs: "+validNeeds, 1)
	if c, err := Parse([]byte(ok)); err != nil {
		t.Errorf("valid needs rejected: %v", err)
	} else if n := c.Tools[0].Platforms["windows/amd64"].Needs; n == nil || n.File != "x.dll" {
		t.Errorf("needs = %+v", n)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(validTool, tt.old) {
				t.Fatalf("test setup: %q not found", tt.old)
			}
			_, err := Parse([]byte(strings.Replace(validTool, tt.old, tt.new, 1)))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseVersionAndIdentity(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	kubectl, _ := c.Tool("kubectl")
	kubelogin, _ := c.Tool("kubelogin")

	tests := []struct {
		name        string
		tool        *Tool
		output      string
		wantVersion string // "" means no version found
	}{
		{"kubectl json", kubectl, `{
  "clientVersion": {
    "major": "1",
    "minor": "34",
    "gitVersion": "v1.34.1",
    "platform": "windows/amd64"
  },
  "kustomizeVersion": "v5.7.1"
}`, "1.34.1"},
		{"kubelogin release", kubelogin, "kubelogin version v1.36.4 (go1.26.6 windows_amd64)\n", "1.36.4"},
		{"kubelogin without version", kubelogin, "kubelogin version  (go1.26.6 windows_amd64)\n", ""},
	}
	for _, tt := range tests {
		got, ok := tt.tool.ParseVersion(tt.output)
		if got != tt.wantVersion || ok != (tt.wantVersion != "") {
			t.Errorf("%s: ParseVersion = %q, %v; want %q", tt.name, got, ok, tt.wantVersion)
		}
	}

	int128Help := "Log in to the OpenID Connect provider.\n\nYou need to set up the OIDC provider, role binding, Kubernetes API server and kubeconfig.\n"
	azureHelp := "login to azure active directory and populate kubeconfig with AAD tokens\n\nUsage:\n  kubelogin [flags]\n"
	if !kubelogin.IsRightProduct(int128Help) {
		t.Error("int128 kubelogin not recognized")
	}
	if kubelogin.IsRightProduct(azureHelp) {
		t.Error("Azure kubelogin accepted as int128")
	}
	if kubelogin.IsRightProduct(int128Help + "\nsee also: Azure Active Directory\n") {
		t.Error("mustNotMatch ignored")
	}
	if !kubectl.IsRightProduct(tests[0].output) {
		t.Error("kubectl not recognized")
	}
}

func TestAccepts(t *testing.T) {
	one, four := 1, 4
	skew := &Tool{Version: "1.37.2", Accept: Accept{ServerMinorSkew: &one}}
	major := &Tool{Version: "4.3.2", Accept: Accept{Major: &four}}
	none := &Tool{Version: "5.8.3"}
	tests := []struct {
		name            string
		tool            *Tool
		version, server string
		want            bool
	}{
		{"skew: same minor", skew, "1.37.0", "v1.37.2", true},
		{"skew: one behind", skew, "1.36.6", "v1.37.2", true},
		{"skew: one ahead", skew, "1.38.0", "v1.37.2", true},
		{"skew: three behind (Docker Desktop on Machine C)", skew, "1.34.1", "v1.37.2", false},
		{"skew: no server, pinned version used", skew, "1.36.6", "", true},
		{"skew: no server, too far from pin", skew, "1.35.10", "", false},
		{"skew: version unknown", skew, "", "v1.37.2", false},
		{"skew: other major", skew, "2.37.0", "v1.37.2", false},
		{"major: Helm 4", major, "4.0.1", "", true},
		{"major: Helm 3", major, "3.19.0", "v1.37.2", false},
		{"major: version unknown", major, "", "", false},
		{"no rule: any version", none, "1.0.0", "", true},
		{"no rule: version unknown", none, "", "", true},
	}
	for _, tt := range tests {
		if got := tt.tool.Accepts(tt.version, tt.server); got != tt.want {
			t.Errorf("%s: Accepts(%q, %q) = %v, want %v", tt.name, tt.version, tt.server, got, tt.want)
		}
	}

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	kubectl, _ := c.Tool("kubectl")
	helm, _ := c.Tool("helm")
	if kubectl.Accept.ServerMinorSkew == nil || *kubectl.Accept.ServerMinorSkew != 1 {
		t.Error("kubectl: want accept.serverMinorSkew 1")
	}
	if helm.Accept.Major == nil || *helm.Accept.Major != 4 {
		t.Error("helm: want accept.major 4")
	}
}

// The outputs follow the formats in each tool's upstream source at the pinned
// version (see the comments in catalog/tools.yaml and DECISIONS.md D42-D43).
func TestExtrasVersionAndIdentity(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	tool := func(id string) *Tool {
		t.Helper()
		x, ok := c.Tool(id)
		if !ok {
			t.Fatalf("tool %q missing", id)
		}
		return x
	}

	versions := []struct {
		id, output, want string // want "" means no version found
	}{
		{"helm", "v4.3.2+g8346aaa\n", "4.3.2"},
		{"helm", "WARNING: a line before the version\nv4.3.2+g8346aaa\n", "4.3.2"},
		{"helm", "v3.19.0+g3d8990f\n", "3.19.0"},
		{"kustomize", "v5.8.3\n", "5.8.3"},
		{"kubeconform", "v0.8.0\n", "0.8.0"},
		{"kubeconform", "development\n", ""},
		{"trivy", "Version: 0.76.2\nVulnerability DB:\n  Version: 2\n  UpdatedAt: 2026-11-22 06:00:00 +0000 UTC\n", "0.76.2"},
		{"trivy", "Vulnerability DB:\n  Version: 2\n", ""},
		{"k9s", "Version              v0.51.0\nCommit               558caaf\nDate                 2026-06-06T05:21:57Z\n", "0.51.0"},
		{"oha", "oha 1.17.0\n", "1.17.0"},
	}
	for _, tt := range versions {
		got, ok := tool(tt.id).ParseVersion(tt.output)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("%s: ParseVersion(%q) = %q, %v; want %q", tt.id, tt.output, got, ok, tt.want)
		}
	}

	helps := map[string]string{
		"helm":        "The Kubernetes package manager\n\nCommon actions for Helm:\n",
		"kustomize":   "\nManages declarative configuration of Kubernetes.\nSee https://sigs.k8s.io/kustomize\n",
		"kubeconform": "Usage: C:\\tools\\kubeconform.exe [OPTION]... [FILE OR FOLDER]...\n  -cache string\n",
		"trivy":       "Scanner for vulnerabilities in container images, file systems, and Git repositories, as well as for configuration issues and hard-coded secrets\n\nUsage:\n",
		"k9s":         "K9s is a CLI to view and manage your Kubernetes clusters.\n\nUsage:\n  k9s [flags]\n",
		"oha":         "Ohayou(おはよう), HTTP load generator, inspired by rakyll/hey with tui animation.\n\nUsage: oha [OPTIONS] <URL>\n",
	}
	for id := range helps {
		for other, help := range helps {
			if got, want := tool(id).IsRightProduct(help), id == other; got != want {
				t.Errorf("%s: IsRightProduct(help of %s) = %v, want %v", id, other, got, want)
			}
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"13.7.8", "12.0", 1},
		{"13.7.8", "13", 1},
		{"13.0", "13", 0},
		{"11.7.10", "12.0", -1},
		{"10.0.19045", "10.0", 1},
		{"v1.36.6", "1.36.6", 0},
		{"1.9", "1.10", -1},
	}
	for _, tt := range tests {
		got, err := CompareVersions(tt.a, tt.b)
		if err != nil || got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, %v; want %d", tt.a, tt.b, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "1..2", "13.x", "Ventura"} {
		if _, err := CompareVersions(bad, "1"); err == nil {
			t.Errorf("CompareVersions(%q): want error", bad)
		}
	}
}

func TestSupportsOS(t *testing.T) {
	a := Artifact{MinOS: "12.0"}
	for v, want := range map[string]bool{"13.7.8": true, "12.0": true, "11.7.10": false} {
		if got, err := a.SupportsOS(v); err != nil || got != want {
			t.Errorf("SupportsOS(%q) = %v, %v; want %v", v, got, err, want)
		}
	}
}
