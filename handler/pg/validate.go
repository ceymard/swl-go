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

	tableErrs []string
}

type validatorOpts struct {
	maxRejects int
	keyLimit   int64
	insertMode bool // false for update mode: absent columns are not written
	checkDups  bool
	emit       func(string)
}

// newTableValidator builds the checkers for the collection's columns. Table-level
// problems (generated columns, missing required columns) are collected in
// tableErrs for the caller to report once.
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
	}
	pos := make(map[string]int, len(cols))
	for i, name := range cols {
		pos[name] = i
		c := meta.column(name)
		if c == nil {
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
				v.tableErrs = append(v.tableErrs, fmt.Sprintf("column %s is NOT NULL without default but the data has no such column", c.Name))
			}
		}
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
}

// check validates one row (1-based number rowNum) and reports it when invalid.
// It returns true when the row is fine.
func (v *tableValidator) check(row coll.Row, rowNum int64) bool {
	var probs []rowProblem
	for i, name := range v.cols {
		cell := row.Cell(i)
		if !v.inTable[i] {
			if cell != nil {
				probs = append(probs, rowProblem{name, "column not in table", "column does not exist in the table; value would be silently dropped"})
			}
			continue
		}
		if cell == nil {
			if v.notNull[i] {
				probs = append(probs, rowProblem{name, "null", "null value in NOT NULL column"})
			}
			continue
		}
		if chk := v.checks[i]; chk != nil {
			if e := chk(cellText(cell)); e != nil {
				probs = append(probs, rowProblem{name, e.Kind, e.Msg})
			}
		}
	}
	if len(row) > len(v.cols) {
		for i := len(v.cols); i < len(row); i++ {
			if row[i] != nil {
				name := fmt.Sprintf("#%d", i)
				if all := v.cs.Columns(); i < len(all) {
					name = all[i].ColumnName
				}
				probs = append(probs, rowProblem{name, "column not in table", "column appeared after the table was opened; value would be silently dropped"})
			}
		}
	}
	if len(probs) == 0 && !v.keysOff {
		probs = v.checkKeys(row, rowNum, probs)
	}
	if len(probs) == 0 {
		return true
	}
	v.report(row, rowNum, probs)
	return false
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
			probs = append(probs, rowProblem{kt.label, "duplicate key", fmt.Sprintf("duplicate unique key, first seen at row %d", first)})
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

func (v *tableValidator) report(row coll.Row, rowNum int64, probs []rowProblem) {
	v.rejected++
	for _, p := range probs {
		k := p.column + ": " + p.kind
		if _, ok := v.counts[k]; !ok {
			v.order = append(v.order, k)
		}
		v.counts[k]++
	}
	if v.maxRejects > 0 && v.printed >= v.maxRejects {
		return
	}
	v.printed++
	parts := make([]string, len(probs))
	for i, p := range probs {
		parts[i] = p.column + ": " + p.msg
	}
	v.emit(fmt.Sprintf("%s[%d] %s  %s", v.table, rowNum, strings.Join(parts, "; "), v.preview(row)))
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

// summary prints per-reason counts once the detailed lines were capped.
func (v *tableValidator) summary() {
	if v.rejected == 0 {
		return
	}
	v.emit(fmt.Sprintf("%s: %d invalid rows", v.table, v.rejected))
	if v.maxRejects > 0 && v.printed >= v.maxRejects && v.rejected > int64(v.printed) {
		for _, k := range v.order {
			v.emit(fmt.Sprintf("  %s: %d", k, v.counts[k]))
		}
	}
}
