// Package ui prints what kube-onramp does: steps, labeled lines, problems,
// progress and the one prompt. Everything shown is also written to the log.
package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/Sten-Qy-Li/kube-onramp/internal/platform"
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
	"github.com/Sten-Qy-Li/kube-onramp/internal/report"
)

// Label is an ASCII status label. These show correctly in old Windows 10
// consoles.
type Label string

// Labels.
const (
	OK    Label = "[OK]"
	Fixed Label = "[FIXED]"
	Warn  Label = "[WARN]"
	Fail  Label = "[FAIL]"
)

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
)

// UI writes to the terminal and the log.
type UI struct {
	mu          sync.Mutex
	out         io.Writer
	in          *bufio.Reader
	log         *report.Logger
	color       bool
	verbose     bool
	tty         bool
	interactive bool
	lastPct     int
	lastBucket  int // last 25% step printed when out is not a terminal
	inProgress  bool
}

// New creates a UI. Color is used only if out is a terminal that supports it
// and neither --no-color nor NO_COLOR is set.
func New(out, in *os.File, log *report.Logger, noColor, verbose bool) *UI {
	u := &UI{out: out, in: bufio.NewReader(in), log: log, verbose: verbose, lastBucket: -1}
	u.tty = platform.IsTerminal(out)
	u.interactive = platform.IsTerminal(in)
	if !noColor && os.Getenv("NO_COLOR") == "" && u.tty {
		u.color = platform.EnableColor(out)
	}
	return u
}

// NewPlain creates a UI without color or a terminal (for tests).
func NewPlain(out io.Writer, in io.Reader, log *report.Logger) *UI {
	return &UI{out: out, in: bufio.NewReader(in), log: log, lastBucket: -1}
}

// Interactive reports whether the user can type an answer.
func (u *UI) Interactive() bool { return u.interactive }

// TTY reports whether the output goes to a terminal (a person is watching).
func (u *UI) TTY() bool { return u.tty }

func (u *UI) paint(code, s string) string {
	if !u.color {
		return s
	}
	return code + s + reset
}

func (u *UI) write(screen, logLine string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.inProgress {
		_, _ = fmt.Fprintln(u.out)
		u.inProgress = false
	}
	_, _ = fmt.Fprint(u.out, screen)
	if u.log != nil && logLine != "" {
		u.log.Printf("%s", logLine)
	}
}

// Step prints "Step N of M: text".
func (u *UI) Step(n, total int, text string) {
	line := fmt.Sprintf("Step %d of %d: %s", n, total, text)
	u.write("\n"+u.paint(bold, line)+"\n", "== "+line)
}

// Item prints one labeled line, like "  [OK]    kubectl 1.36.6".
func (u *UI) Item(l Label, text string) {
	colors := map[Label]string{OK: green, Fixed: cyan, Warn: yellow, Fail: red}
	pad := strings.Repeat(" ", 8-len(l))
	u.write("  "+u.paint(colors[l], string(l))+pad+text+"\n", string(l)+" "+text)
}

// Say prints plain lines, indented.
func (u *UI) Say(lines ...string) {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString("  " + l + "\n")
	}
	u.write(b.String(), strings.Join(lines, "\n"))
}

// Title prints an unindented bold line.
func (u *UI) Title(text string) {
	u.write("\n"+u.paint(bold, text)+"\n", "== "+text)
}

// Blank prints an empty line.
func (u *UI) Blank() { u.write("\n", "") }

// Debug writes to the log, and to the screen with --verbose.
func (u *UI) Debug(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if u.verbose {
		u.write("    "+msg+"\n", msg)
		return
	}
	if u.log != nil {
		u.log.Printf("%s", msg)
	}
}

// Problem prints an error or warning with its next steps.
func (u *UI) Problem(p *problem.Problem) {
	color := red
	if p.IsWarning() {
		color = yellow
	}
	var b strings.Builder
	b.WriteString("\n  " + u.paint(color+bold, p.Code) + ": ")
	for i, l := range p.Lines {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(l + "\n")
	}
	for i, n := range p.Next {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, n)
	}
	logText := p.Code + ": " + strings.Join(p.Lines, " ")
	if p.Cause != nil {
		logText += "\ncause: " + p.Cause.Error()
	}
	u.write(b.String(), logText)
}

// Progress shows a download percentage. On a terminal it updates one line;
// otherwise it prints every 25%.
func (u *UI) Progress(text string, done, total int64) {
	if total <= 0 {
		return
	}
	pct := int(done * 100 / total)
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.tty {
		if pct == u.lastPct && u.inProgress {
			return
		}
		_, _ = fmt.Fprintf(u.out, "\r  %s %3d%%", text, pct)
		u.inProgress = pct < 100
		if pct >= 100 {
			_, _ = fmt.Fprintln(u.out)
		}
	} else if bucket := pct / 25; bucket > u.lastBucket {
		_, _ = fmt.Fprintf(u.out, "  %s %d%%\n", text, bucket*25)
		u.lastBucket = bucket
	}
	u.lastPct = pct
	if pct >= 100 {
		u.lastPct = 0
		u.lastBucket = -1
		u.inProgress = false
	}
}

// ErrNoInput means there is no terminal to read from.
var ErrNoInput = errors.New("no input")

// WaitEnter shows a prompt and waits for Enter.
func (u *UI) WaitEnter(prompt string) error {
	u.write("\n  "+u.paint(bold, prompt)+" ", prompt)
	_, err := u.in.ReadString('\n')
	if errors.Is(err, io.EOF) {
		return ErrNoInput
	}
	return err
}

// Ask shows a prompt and returns one line of input.
func (u *UI) Ask(prompt string) (string, error) {
	u.write("  "+u.paint(bold, prompt)+" ", prompt)
	line, err := u.in.ReadString('\n')
	if errors.Is(err, io.EOF) && line == "" {
		return "", ErrNoInput
	}
	line = strings.TrimSpace(line)
	if u.log != nil {
		u.log.Printf("answer: %q", line)
	}
	return line, nil
}
