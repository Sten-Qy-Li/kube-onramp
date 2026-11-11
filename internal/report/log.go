package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// KeepLogs is how many run logs (and reports) are kept.
const KeepLogs = 10

const tailLines = 200

// Logger writes a verbose, redacted log of one run. It also keeps the last
// lines in memory for the error report. A Logger without a file still keeps
// the tail, so a broken log folder never stops the run.
type Logger struct {
	mu   sync.Mutex
	f    *os.File
	path string
	red  *Redactor
	tail []string
	now  func() time.Time
}

// OpenLog creates logs/<timestamp>.log in dir and removes old logs.
func OpenLog(dir string, now time.Time, red *Redactor) (*Logger, error) {
	l := &Logger{red: red, now: time.Now}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return l, err
	}
	l.path = filepath.Join(dir, now.Format("20060102-150405")+".log")
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		l.path = ""
		return l, err
	}
	l.f = f
	Prune(dir, ".log", KeepLogs)
	return l, nil
}

// NewMemoryLogger keeps lines only in memory (for tests and early start-up).
func NewMemoryLogger(red *Redactor) *Logger {
	return &Logger{red: red, now: time.Now}
}

// Path is the log file, or "" if there is none.
func (l *Logger) Path() string { return l.path }

// Redactor returns the redactor used by this log.
func (l *Logger) Redactor() *Redactor { return l.red }

// Printf logs one message. Multi-line messages become several lines.
func (l *Logger) Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if l.red != nil {
		msg = l.red.Redact(msg)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	stamp := l.now().Format("15:04:05.000")
	for _, line := range strings.Split(strings.TrimRight(msg, "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		l.tail = append(l.tail, line)
		if l.f != nil {
			_, _ = fmt.Fprintf(l.f, "%s %s\n", stamp, line)
		}
	}
	if len(l.tail) > tailLines {
		l.tail = l.tail[len(l.tail)-tailLines:]
	}
}

// Tail returns the last n redacted log lines.
func (l *Logger) Tail(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > len(l.tail) {
		n = len(l.tail)
	}
	return append([]string(nil), l.tail[len(l.tail)-n:]...)
}

// Close closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// Prune keeps the newest keep files in dir that end with suffix.
// File names start with a sortable timestamp (logs: "<ts>.log",
// reports: "report-<ts>.txt").
func Prune(dir, suffix string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), suffix) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}
