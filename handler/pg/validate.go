package pg

import (
	"fmt"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/jsonx"
)

const maxRowPreview = 200

// keyTracker detects duplicate values of one unique index within the incoming
// data. Keys are kept in full (no hashing) so a collision can never cause a
// false reject.
type keyTracker struct {
	label        string
	idx          []int
	nullsNotDist bool
	seen         map[string]int64 // key -> row number where first seen
}

// tableValidator checks rows against what introspection learned about the
// target table, before anything is sent to Postgres.
type tableValidator struct {
	table   string
	cols    []string
	cs      *coll.ColumnSet
	checks  []textChecker
	notNull []bool
	inTable []bool
	keys    []*keyTracker
	emit    func(string)

	maxRejects int // detailed lines per table; 0 = unlimited
	printed    int
	rejected   int64
	counts     map[string]int64
	order      []string

	keyMem   int64
	keyLimit int64 // bytes; 0 = unlimited
	keysOff  bool

	// Findings Postgres may well accept (triggers can fill NULLs and rewrite
	// keys, columns discovered after Open are dropped silently) are warnings:
	// reported, but they never fail the load.
	warnedRows  int64
	warnPrinted int
	warnCounts  map[string]int64
	warnOrder   []string
	lateWarned  map[string]bool

	tableErrs  []string // certain failures, reported once per table
	tableWarns []string // reported once per table
}

type validatorOpts struct {
	maxRejects int
	keyLimit   int64
	insertMode bool // false for update mode: absent columns are not written
	checkDups  bool
	emit       func(string)
}

// newTableValidator builds the checkers for the collection's columns. Table-level
// problems are collected for the caller to report once: tableErrs for what
// Postgres will certainly refuse (generated columns), tableWarns for the rest
// (unknown columns, missing NOT NULL columns a trigger might fill).
func newTableValidator(table string, meta *pgTableMeta, cols []string, cs *coll.ColumnSet, o validatorOpts) *tableValidator {
	v := &tableValidator{
		table:      table,
		cols:       cols,
		cs:         cs,
		checks:     make([]textChecker, len(cols)),
		notNull:    make([]bool, len(cols)),
		inTable:    make([]bool, len(cols)),
		emit:       o.emit,
		maxRejects: o.maxRejects,
		keyLimit:   o.keyLimit,
		counts:     map[string]int64{},
		warnCounts: map[string]int64{},
		lateWarned: map[string]bool{},
	}
	var unknown []string
	pos := make(map[string]int, len(cols))
	for i, name := range cols {
		pos[name] = i
		c := meta.column(name)
		if c == nil {
			unknown = append(unknown, name)
			continue
		}
		v.inTable[i] = true
		v.notNull[i] = c.NotNull
		v.checks[i] = checkerFor(*c)
		if c.Generated {
			v.tableErrs = append(v.tableErrs, fmt.Sprintf("column %s is GENERATED ALWAYS: Postgres refuses explicit values", name))
		}
	}
	if o.insertMode {
		for _, c := range meta.Columns {
			if _, ok := pos[c.Name]; !ok && c.NotNull && !c.HasDef && !c.Generated {
				v.tableWarns = append(v.tableWarns, fmt.Sprintf("column %s is NOT NULL without default but the data has no such column (a trigger may fill it)", c.Name))
			}
		}
	}
	if len(unknown) > 0 {
		// The INSERT names every collection column, so Postgres refuses these.
		v.tableErrs = append(v.tableErrs, fmt.Sprintf("columns do not exist in the table: %s", strings.Join(unknown, ", ")))
	}
	if o.checkDups {
	uniques:
		for _, u := range meta.Uniques {
			kt := &keyTracker{label: strings.Join(u.Columns, ", "), nullsNotDist: u.NullsNotDist, seen: map[string]int64{}}
			for _, name := range u.Columns {
				i, ok := pos[name]
				if !ok || !v.inTable[i] {
					continue uniques
				}
				kt.idx = append(kt.idx, i)
			}
			v.keys = append(v.keys, kt)
		}
	}
	return v
}

type rowProblem struct {
	column string
	kind   string
	msg    string
	soft   bool // a trigger could make Postgres accept it: warning only
}

// check validates one row (1-based number rowNum). It returns false only for
// a certain rejection; warnings are reported but the row is still loaded.
func (v *tableValidator) check(row coll.Row, rowNum int64) bool {
	var probs []rowProblem
	hard := false
	for i, name := range v.cols {
		cell := row.Cell(i)
		if !v.inTable[i] {
			continue // reported once per table
		}
		if cell == nil {
			if v.notNull[i] {
				probs = append(probs, rowProblem{name, "null", "null value in NOT NULL column (a trigger may fill it)", true})
			}
			continue
		}
		if chk := v.checks[i]; chk != nil {
			if e := chk(cellText(cell)); e != nil {
				probs = append(probs, rowProblem{name, e.Kind, e.Msg, false})
				hard = true
			}
		}
	}
	for i := len(v.cols); i < len(row); i++ {
		if row[i] == nil {
			continue
		}
		name := fmt.Sprintf("#%d", i)
		if all := v.cs.Columns(); i < len(all) {
			name = all[i].ColumnName
		}
		if !v.lateWarned[name] {
			v.lateWarned[name] = true
			v.emit(fmt.Sprintf("warning: %s: column %s appeared after the table was opened, its values are dropped", v.table, name))
		}
	}
	if !hard && !v.keysOff {
		probs = v.checkKeys(row, rowNum, probs)
	}
	if len(probs) == 0 {
		return true
	}
	v.report(row, rowNum, probs, hard)
	return !hard
}

func (v *tableValidator) checkKeys(row coll.Row, rowNum int64, probs []rowProblem) []rowProblem {
	var sb strings.Builder
	for _, kt := range v.keys {
		sb.Reset()
		hasNull := false
		for _, i := range kt.idx {
			cell := row.Cell(i)
			if cell == nil {
				hasNull = true
				sb.WriteString("\x00N")
				continue
			}
			t := cellText(cell)
			fmt.Fprintf(&sb, "%d:%s", len(t), t)
		}
		if hasNull && !kt.nullsNotDist {
			continue
		}
		key := sb.String()
		if first, dup := kt.seen[key]; dup {
			probs = append(probs, rowProblem{kt.label, "duplicate key", fmt.Sprintf("duplicate unique key, first seen at row %d", first), true})
			continue
		}
		kt.seen[key] = rowNum
		v.keyMem += int64(len(key)) + 64
		if v.keyLimit > 0 && v.keyMem > v.keyLimit {
			v.keysOff = true
			for _, k := range v.keys {
				k.seen = nil
			}
			v.emit(fmt.Sprintf("%s: duplicate-key tracking stopped after %d MB; Postgres will catch any remaining duplicates", v.table, v.keyLimit>>20))
			return probs
		}
	}
	return probs
}

func (v *tableValidator) report(row coll.Row, rowNum int64, probs []rowProblem, hard bool) {
	counts, order, printed, prefix := v.warnCounts, &v.warnOrder, &v.warnPrinted, "warning: "
	if hard {
		v.rejected++
		counts, order, printed, prefix = v.counts, &v.order, &v.printed, ""
	} else {
		v.warnedRows++
	}
	for _, p := range probs {
		k := p.column + ": " + p.kind
		if _, ok := counts[k]; !ok {
			*order = append(*order, k)
		}
		counts[k]++
	}
	if v.maxRejects > 0 && *printed >= v.maxRejects {
		return
	}
	*printed++
	parts := make([]string, len(probs))
	for i, p := range probs {
		parts[i] = p.column + ": " + p.msg
	}
	v.emit(fmt.Sprintf("%s%s[%d] %s  %s", prefix, v.table, rowNum, strings.Join(parts, "; "), v.preview(row)))
}

func (v *tableValidator) preview(row coll.Row) string {
	m := make(map[string]any, len(v.cols))
	for i, name := range v.cols {
		m[name] = row.Cell(i)
	}
	b, err := jsonx.Marshal(m)
	if err != nil {
		return ""
	}
	if len(b) > maxRowPreview {
		return string(b[:maxRowPreview]) + "…"
	}
	return string(b)
}

// summary prints totals, plus per-reason counts once detailed lines were capped.
func (v *tableValidator) summary() {
	if v.rejected > 0 {
		v.emit(fmt.Sprintf("%s: %d invalid rows", v.table, v.rejected))
		if v.maxRejects > 0 && v.printed >= v.maxRejects && v.rejected > int64(v.printed) {
			for _, k := range v.order {
				v.emit(fmt.Sprintf("  %s: %d", k, v.counts[k]))
			}
		}
	}
	if v.warnedRows > 0 {
		v.emit(fmt.Sprintf("warning: %s: %d rows with warnings", v.table, v.warnedRows))
		if v.maxRejects > 0 && v.warnPrinted >= v.maxRejects && v.warnedRows > int64(v.warnPrinted) {
			for _, k := range v.warnOrder {
				v.emit(fmt.Sprintf("  %s: %d", k, v.warnCounts[k]))
			}
		}
	}
}
