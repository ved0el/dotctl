package console

import (
	"bytes"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	tests := []struct {
		name    string
		verbose bool
		emit    func(*Logger)
		want    string
		absent  string
	}{
		{name: "step", emit: func(l *Logger) { l.Step("doing %s", "x") }, want: "[~] doing x"},
		{name: "ok", emit: func(l *Logger) { l.OK("done") }, want: "[+] done"},
		{name: "warn", emit: func(l *Logger) { l.Warn("careful") }, want: "[!] careful"},
		{name: "plan", emit: func(l *Logger) { l.Plan("link", "~/.zshrc") }, want: "[dry-run] would link ~/.zshrc"},
		{name: "debug shown when verbose", verbose: true, emit: func(l *Logger) { l.Debug("detail") }, want: "detail"},
		{name: "debug hidden when quiet", verbose: false, emit: func(l *Logger) { l.Debug("detail") }, absent: "detail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := New(&buf, tt.verbose)
			tt.emit(l)
			got := buf.String()
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("output %q does not contain %q", got, tt.want)
			}
			if tt.absent != "" && strings.Contains(got, tt.absent) {
				t.Errorf("output %q should not contain %q", got, tt.absent)
			}
		})
	}
}
