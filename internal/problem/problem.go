// Package problem defines the error and warning codes that users see.
// Every code has a fixed message in plain B1 English and clear next steps.
// Technical details go into Cause, which is only written to the log.
package problem

import (
	"fmt"
	"strings"
)

// Problem is an error (E-*) or warning (W-*) with a user-facing message.
type Problem struct {
	Code  string
	Lines []string // what happened
	Next  []string // what to do, shown as a numbered list
	Cause error    // technical detail for the log only
	// Trying says what kube-onramp was doing, for the error report,
	// for example "log in to the cluster through the browser (step 6 of 8)".
	Trying string
}

func (p *Problem) Error() string {
	msg := p.Code
	if len(p.Lines) > 0 {
		msg += ": " + p.Lines[0]
	}
	if p.Cause != nil {
		msg += " (" + p.Cause.Error() + ")"
	}
	return msg
}

func (p *Problem) Unwrap() error { return p.Cause }

// IsWarning reports whether this is a W-* code.
func (p *Problem) IsWarning() bool { return strings.HasPrefix(p.Code, "W-") }

func newP(code string, cause error, lines []string, next ...string) *Problem {
	return &Problem{Code: code, Lines: lines, Next: next, Cause: cause}
}

// PreOS: the operating system or its version is not supported.
func PreOS(osName, supported string) *Problem {
	return newP("E-PRE-OS", nil,
		[]string{"kube-onramp does not support this computer: " + osName + ".", "It works on: " + supported + "."},
		"Use a computer with one of these systems, or ask a TA for help.")
}

// PreWSL: running inside WSL instead of on Windows itself.
func PreWSL() *Problem {
	return newP("E-PRE-WSL", nil,
		[]string{"You started kube-onramp inside WSL (Linux on Windows).",
			"Your browser and your normal terminals run on Windows, so the setup must happen on Windows too."},
		"Open PowerShell on Windows (not WSL).",
		"Paste the install line for Windows from the README there.")
}

// PreDisk: not enough free disk space.
func PreDisk(freeBytes int64, where string) *Problem {
	return newP("E-PRE-DISK", nil,
		[]string{fmt.Sprintf("There is not enough free disk space. kube-onramp needs 1 GB, but only %s is free.", HumanBytes(freeBytes))},
		"Free some space on the disk with this folder: "+where,
		"Run kube-onramp again.")
}

// PreNet: a host that the plan needs can't be reached.
func PreNet(host string, cause error) *Problem {
	return newP("E-PRE-NET", cause,
		[]string{"kube-onramp cannot reach " + host + ".", "Nothing was changed on your computer."},
		"Check that you are online. Open https://"+host+" in your browser to test it.",
		"If you use a VPN or a company network, try another network.",
		"Run kube-onramp again.")
}

// PrePorts: both login ports are busy.
func PrePorts() *Problem {
	return newP("E-PRE-PORTS", nil,
		[]string{"The login needs a free network port on your computer: 8000 or 18000.",
			"Both are used by other programs right now."},
		"Close programs that may use these ports (for example a local web server or Docker containers).",
		"Run kube-onramp again.")
}

// Download: a download failed.
func Download(what string, cause error) *Problem {
	return newP("E-DL", cause,
		[]string{"The download of " + what + " did not work."},
		"Check that you are online.",
		"Run kube-onramp again. It continues where it stopped.")
}

// SHA: a download did not match its pinned checksum.
func SHA(what string, cause error) *Problem {
	return newP("E-SHA", cause,
		[]string{"The downloaded file for " + what + " is not the expected file (the checksum is wrong).",
			"kube-onramp did not install it."},
		"Run kube-onramp again. A broken download often works the second time.",
		"If it happens again, a proxy or antivirus program may change downloads. Send the report to a TA.")
}

// Extract: an archive could not be unpacked.
func Extract(what string, cause error) *Problem {
	return newP("E-EXTRACT", cause,
		[]string{"The downloaded archive for " + what + " could not be unpacked."},
		"Run kube-onramp again.",
		"If it happens again, an antivirus program may block the file. Send the report to a TA.")
}

// Install: a file could not be put into kube-onramp's folder.
func Install(what string, cause error) *Problem {
	return newP("E-INSTALL", cause,
		[]string{"kube-onramp could not put " + what + " into its own folder."},
		"Close other terminal windows that may use it.",
		"Run kube-onramp again.",
		"If it happens again, an antivirus program may block the file. Send the report to a TA.")
}

// Path: PATH could not be updated.
func Path(cause error) *Problem {
	return newP("E-PATH", cause,
		[]string{"kube-onramp could not update your PATH (the list of folders where the terminal looks for programs)."},
		"Run kube-onramp again.",
		"If it happens again, send the report to a TA.")
}

// Kubeconfig: the kubeconfig could not be read or written.
func Kubeconfig(file string, cause error) *Problem {
	return newP("E-KCFG", cause,
		[]string{"kube-onramp could not read or write your Kubernetes settings file:", "  " + file},
		"Close programs that may have this file open (for example an editor or Lens).",
		"Run kube-onramp again.")
}

// LoginTimeout: the browser login did not finish in time.
func LoginTimeout() *Problem {
	return newP("E-LOGIN-TIMEOUT", nil,
		[]string{"The login did not finish in time."},
		"Run kube-onramp again.",
		"When the browser opens, log in, then come back to this window.",
		"If no browser opens, look for a link in this window and open it yourself.")
}

// LoginRejected: the cluster did not accept the login.
func LoginRejected(cause error) *Problem {
	return newP("E-LOGIN-REJECTED", cause,
		[]string{"You logged in, but the cluster did not accept the login."},
		"Check that you used your own university account.",
		"Run kube-onramp again.",
		"If it happens again, ask a TA to check your access in ETAIS.")
}

// LoginFailed: the login failed for another reason.
func LoginFailed(cause error) *Problem {
	return newP("E-LOGIN-FAILED", cause,
		[]string{"The login did not work."},
		"Run kube-onramp again.",
		"If it happens again, send the report to a TA.")
}

// ClusterUnreachable: the API server or the login server can't be reached.
func ClusterUnreachable(cause error) *Problem {
	return newP("E-CLUSTER-UNREACHABLE", cause,
		[]string{"kube-onramp cannot reach the cluster from your network."},
		"Check that you are online.",
		"If you use a VPN or a company network, try another network.",
		"Run kube-onramp check again.")
}

// AccessForbidden: logged in, but no access to the staging namespace.
func AccessForbidden(namespace string) *Problem {
	return newP("E-ACCESS-FORBIDDEN", nil,
		[]string{"You are logged in, but you cannot see the namespace " + namespace + ".",
			"A wrong team name and \"no access yet\" look the same."},
		"Check your team name. Run again with: kube-onramp --team <your-team>",
		"If the name is right, ask the teammate who created the namespace in ETAIS, or a TA, to give you access.")
}

// Internal: a bug in kube-onramp (recovered panic or unexpected state).
func Internal(cause error) *Problem {
	return newP("E-INTERNAL", cause,
		[]string{"Something went wrong inside kube-onramp. This is a bug, not your fault."},
		"Run kube-onramp again.",
		"If it happens again, send the report to the kube-onramp maintainers or a TA.")
}

// Shadow: another copy wins on PATH in a new terminal.
func Shadow(name, path string, acceptable bool, hint string) *Problem {
	lines := []string{"In a new terminal, \"" + name + "\" starts another copy:", "  " + path}
	if acceptable {
		lines = append(lines, "That copy works with the cluster, so you can keep it.")
		return newP("W-SHADOW", nil, lines)
	}
	lines = append(lines, "That copy is too old or not the right program.")
	return newP("W-SHADOW", nil, lines, hint)
}

// Skew: kubectl and the server are more than one minor version apart.
func Skew(client, server string) *Problem {
	return newP("W-SKEW", nil,
		[]string{"Your kubectl (" + client + ") and the cluster (" + server + ") are far apart in version. Some commands may not work."},
		"Run kube-onramp to install the right kubectl.")
}

// ProdNotYet: no access to the prod namespace yet (normal before Phase 4).
func ProdNotYet(namespace string) *Problem {
	return newP("W-PROD-NOT-YET", nil,
		[]string{"You cannot see the prod namespace " + namespace + " yet.",
			"This is normal before Phase 4 of the course."})
}

// Extra: an optional tool failed. It never blocks the core.
func Extra(tool string, cause error) *Problem {
	return newP("W-EXTRA-"+strings.ToUpper(tool), cause,
		[]string{"The extra tool " + tool + " could not be installed. Your cluster access is not affected."},
		"Run kube-onramp again later to try once more.")
}

// HumanBytes formats a byte count for people: "61 MB".
func HumanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", (n+(1<<19))>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", (n+(1<<9))>>10)
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}
