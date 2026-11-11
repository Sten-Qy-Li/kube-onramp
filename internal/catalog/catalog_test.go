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
		{"duplicate", "tools:\n", "tools:\n  - {id: demo}\n", "twice"},
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
