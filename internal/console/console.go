// Package console provides leveled console output and dry-run rendering.
//
// Construct a Logger with New and pass it explicitly; there is no package-level
// global, so output destination and verbosity stay injectable and testable.
package console

import (
	"fmt"
	"io"
)

// Logger writes human-facing progress output.
type Logger struct {
	w       io.Writer
	verbose bool
}

// New returns a Logger writing to w. When verbose is false, Debug is suppressed.
func New(w io.Writer, verbose bool) *Logger {
	return &Logger{w: w, verbose: verbose}
}

// Step reports an action in progress.
func (l *Logger) Step(format string, a ...any) { l.printf("[~] ", format, a...) }

// OK reports a successful action.
func (l *Logger) OK(format string, a ...any) { l.printf("[+] ", format, a...) }

// Warn reports a non-fatal problem.
func (l *Logger) Warn(format string, a ...any) { l.printf("[!] ", format, a...) }

// Debug reports detail shown only in verbose mode.
func (l *Logger) Debug(format string, a ...any) {
	if l.verbose {
		l.printf("    ", format, a...)
	}
}

// Plan reports an action that would run, without performing it (dry-run).
func (l *Logger) Plan(action, target string) {
	_, _ = fmt.Fprintf(l.w, "[dry-run] would %s %s\n", action, target)
}

func (l *Logger) printf(prefix, format string, a ...any) {
	_, _ = fmt.Fprintf(l.w, prefix+format+"\n", a...)
}
