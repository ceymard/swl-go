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
		for batch, err := range c.Rows {
			if err != nil {
				return err
			}
			for _, row := range batch {
				n++
				if err := printRow(out, current, n, row, colorize); err != nil {
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
	return printRow(os.Stderr, c.Name, 0, row, style.Enabled(os.Stderr))
}

// printRow formats one row: "collection:N key: value, ..." with colors when enabled.
func printRow(out io.Writer, collection string, num int, row coll.Row, colorize bool) error {
	if num > 0 {
		prefix := style.Collection(collection, colorize) + style.Sep(":", colorize) + style.LineNum(strconv.Itoa(num), colorize) + " "
		if _, err := io.WriteString(out, prefix); err != nil {
			return errs.Wrap(err, "debug write")
		}
	}
	if err := printValue(out, row, colorize, true); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out)
	return err
}
