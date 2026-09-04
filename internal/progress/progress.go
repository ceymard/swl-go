// Package progress provides aligned row-count and handler logging for sources and sinks.
//
// Row counts are tracked on the stream (exact rows passed), not via database COUNT(*).
// Sinks log periodic throughput at verbose ≥ 2; both roles log a final line per collection
// at verbose ≥ 1 (swl2: emitted » / received «).
package progress

import (
	"fmt"
	"strings"
	"time"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/internal/style"
)

// Role distinguishes source vs sink logging colors and row-count labels.
type Role int

const (
	Source Role = iota
	Sink
)

const tickInterval = time.Second

// Handler writes role-colored handler diagnostics (connection, DDL, …).
type Handler struct {
	log  *msg.Log
	role Role
	name string
}

// NewHandler builds a handler logger from a pipeline stage id (e.g. pg-src → pg).
func NewHandler(log *msg.Log, role Role, stageID string) *Handler {
	name := stageID
	name = strings.TrimSuffix(name, "-src")
	name = strings.TrimSuffix(name, "-sink")
	return &Handler{log: log, role: role, name: name}
}

// Log prints a line prefixed with a colored handler name (postgres », sqlite «, …).
func (h *Handler) Log(level int, args ...any) {
	if h == nil || h.log == nil || h.log.Verbose < level {
		return
	}
	colorize := style.Enabled(h.log.Out)
	fmt.Fprintln(h.log.Out, h.prefix(colorize), formatArgs(args...))
}

// Highlight returns s colored as a source or sink target (URI, path, …).
func (h *Handler) Highlight(s string) string {
	if h == nil || h.log == nil {
		return s
	}
	colorize := style.Enabled(h.log.Out)
	if h.role == Source {
		return style.SigilSource(s, colorize)
	}
	return style.SigilSink(s, colorize)
}

func (h *Handler) prefix(colorize bool) string {
	label := h.name
	if h.role == Source {
		return style.SigilSource(label+" »", colorize)
	}
	return style.SigilSink(label+" «", colorize)
}

func formatArgs(args ...any) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprint(a)
	}
	return strings.Join(parts, " ")
}

// Track wraps in and logs exact row counts as collections are consumed.
func Track(log *msg.Log, role Role, in coll.Stream) coll.Stream {
	if in == nil {
		return in
	}
	return func(yield func(coll.Collection, error) bool) {
		var tr tracker
		tr.log = log
		tr.role = role
		defer tr.end()

		for c, err := range in {
			if err != nil {
				if !yield(c, err) {
					return
				}
				continue
			}
			tr.begin(c.Name)
			rows := func(yieldBatch func([]coll.Row, error) bool) {
				for batch, err := range c.Rows {
					if err != nil {
						yieldBatch(batch, err)
						return
					}
					tr.add(int64(len(batch)))
					if !yieldBatch(batch, nil) {
						return
					}
				}
				tr.end()
			}
			if !yield(coll.Collection{Name: c.Name, Columns: c.Columns, Rows: rows}, nil) {
				return
			}
		}
	}
}

type tracker struct {
	log        *msg.Log
	role       Role
	collection string
	count      int64
	lastAt     time.Time
	lastCount  int64
}

func (t *tracker) begin(name string) {
	t.end()
	t.collection = name
	t.count = 0
	t.lastAt = time.Now()
	t.lastCount = 0
}

func (t *tracker) add(n int64) {
	t.count += n
	if t.log == nil || t.log.Verbose < 2 || t.role != Sink {
		return
	}
	now := time.Now()
	if now.Sub(t.lastAt) >= tickInterval {
		t.logPeriodic(now)
	}
}

func (t *tracker) end() {
	if t.collection == "" {
		return
	}
	if t.log != nil && t.log.Verbose >= 1 {
		t.logFinal()
	}
	t.collection = ""
	t.count = 0
}

func (t *tracker) logPeriodic(now time.Time) {
	elapsed := now.Sub(t.lastAt).Seconds()
	if elapsed <= 0 {
		return
	}
	colorize := style.Enabled(t.log.Out)
	rate := int64(float64(t.count-t.lastCount) / elapsed / 1000)
	if rate < 0 {
		rate = 0
	}
	collName := style.Collection(t.collection, colorize)
	countStr := style.Number(fmt.Sprint(t.count), colorize)
	fmt.Fprintln(t.log.Out, collName, countStr, "rows handled so far -", rate, "Krows/secs")
	t.lastAt = now
	t.lastCount = t.count
}

func (t *tracker) logFinal() {
	colorize := style.Enabled(t.log.Out)
	collName := style.Collection(t.collection, colorize)
	countStr := style.Number(fmt.Sprint(t.count), colorize)
	var arrow string
	if t.role == Source {
		arrow = style.SigilSource("emitted »", colorize)
	} else {
		arrow = style.SigilSink("received «", colorize)
	}
	fmt.Fprintln(t.log.Out, collName, arrow, countStr, "lines")
}
