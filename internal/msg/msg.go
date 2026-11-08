// Package msg provides progress/logging to stderr outside the data stream.
//
// swl2 emitted log lines as side-channel messages; we keep that separation so
// coll.Stream stays pure rows/collections with errors as Go error returns.
package msg

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Log writes handler progress and diagnostics when Verbose is high enough.
type Log struct {
	Out     io.Writer
	Verbose int
}

// New creates a Log writing to stderr at the given verbosity threshold.
func New(verbose int) *Log {
	return &Log{Out: os.Stderr, Verbose: verbose}
}

// Emit logs a structured handler message (origin → target [type] text).
func (l *Log) Emit(origin, target, typ, text string) {
	if l == nil {
		return
	}
	fmt.Fprintf(l.Out, "%s → %s [%s] %s\n", origin, target, typ, text)
}

// Log prints args when l.Verbose >= level (0 = always, 2 = default progress).
func (l *Log) Log(level int, args ...any) {
	if l == nil || l.Verbose < level {
		return
	}
	fmt.Fprintln(l.Out, args...)
}

// FormatDuration renders d as "NNNms" under a second, "SsMMMms" under a minute,
// and "MmSsMMMms" beyond.
func FormatDuration(d time.Duration) string {
	ms := d.Milliseconds()
	switch {
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%ds%dms", ms/1000, ms%1000)
	default:
		return fmt.Sprintf("%dm%ds%dms", ms/60_000, ms/1000%60, ms%1000)
	}
}
