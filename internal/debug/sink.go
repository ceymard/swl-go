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
)

// Sink drains the stream and prints every row to stderr.
func Sink(verbose int, in coll.Stream) error {
	out := os.Stderr
	var current string
	var n int
	for c, err := range in {
		if err != nil {
			return err
		}
		current = c.Name
		n = 0
		for row, err := range c.Rows {
			if err != nil {
				return err
			}
			n++
			if err := printRow(out, current, n, row); err != nil {
				return err
			}
		}
	}
	_ = verbose // reserved for future formatted output levels
	return nil
}

// PrintRow prints one row (used by -p passthrough tee in runner).
func PrintRow(verbose int, c coll.Collection, row coll.Row) error {
	_ = verbose
	return printRow(os.Stderr, c.Name, 0, row)
}

// printRow formats a single row as "collection:N key: value, ...".
func printRow(out io.Writer, collection string, num int, row coll.Row) error {
	if num > 0 {
		_, err := fmt.Fprintf(out, "%s:%d ", collection, num)
		if err != nil {
			return errs.Wrap(err, "debug write")
		}
	}
	first := true
	for k, v := range row {
		if !first {
			_, _ = fmt.Fprint(out, ", ")
		}
		first = false
		_, _ = fmt.Fprintf(out, "%s: %s", k, formatValue(v))
	}
	_, err := fmt.Fprintln(out)
	return err
}

// formatValue renders a cell for human-readable debug output.
func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		if x == "" {
			return "''"
		}
		return strconv.Quote(x)
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return fmt.Sprint(v)
	}
}
