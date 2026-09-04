// Package debug implements the default sink when no terminal :: target is given.
//
// Prints collection:name rownum key: value pairs to stderr — same role as swl2
// debug output when stdout would otherwise be empty.
package debug

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/style"
)

// Sink drains the stream and prints every row to stderr.
func Sink(verbose int, in coll.Stream) error {
	return sinkTo(os.Stderr, verbose, in)
}

func sinkTo(out io.Writer, verbose int, in coll.Stream) error {
	colorize := style.Enabled(out)
	var current string
	var n int
	for c, err := range in {
		if err != nil {
			return err
		}
		current = c.Name
		n = 0
		var names []string
		for batch, err := range c.Rows {
			if err != nil {
				return err
			}
			for _, row := range batch {
				n++
				if c.Columns != nil && len(names) != c.Columns.Len() {
					names = columnNames(c.Columns)
				}
				if err := printRow(out, current, n, names, row, colorize); err != nil {
					return err
				}
			}
		}
	}
	_ = verbose
	return nil
}

// PrintRow prints one row (used by -p passthrough tee in runner).
func PrintRow(verbose int, c coll.Collection, row coll.Row) error {
	_ = verbose
	var names []string
	if c.Columns != nil {
		names = columnNames(c.Columns)
	}
	return printRow(os.Stderr, c.Name, 0, names, row, style.Enabled(os.Stderr))
}

func columnNames(cs *coll.ColumnSet) []string {
	cols := cs.Columns()
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.ColumnName
	}
	return names
}

// printRow formats one row: "collection:N key: value, ..." with colors when enabled.
func printRow(out io.Writer, collection string, num int, names []string, row coll.Row, colorize bool) error {
	if num > 0 {
		prefix := style.Collection(collection, colorize) + style.Sep(":", colorize) + style.LineNum(strconv.Itoa(num), colorize) + " "
		if _, err := io.WriteString(out, prefix); err != nil {
			return errs.Wrap(err, "debug write")
		}
	}
	if err := printPositionalRow(out, names, row, colorize); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out)
	return err
}
