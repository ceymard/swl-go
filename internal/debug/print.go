package debug

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ceymard/swl-go/internal/style"
)

// printValue renders v with type-aware colors (swl2 print_value).
func printValue(out io.Writer, v any, colorize bool, outside bool) error {
	switch x := v.(type) {
	case nil:
		_, err := io.WriteString(out, style.Null(colorize))
		return err
	case string:
		s := x
		if s == "" {
			s = "''"
		} else {
			s = strings.ReplaceAll(s, "\n", `\n`)
			s = strconv.Quote(s)
		}
		_, err := io.WriteString(out, style.String(s, colorize))
		return err
	case bool:
		_, err := io.WriteString(out, style.Bool(strconv.FormatBool(x), colorize))
		return err
	case float64:
		_, err := io.WriteString(out, style.Number(strconv.FormatFloat(x, 'f', -1, 64), colorize))
		return err
	case float32:
		_, err := io.WriteString(out, style.Number(strconv.FormatFloat(float64(x), 'f', -1, 32), colorize))
		return err
	case int:
		_, err := io.WriteString(out, style.Number(strconv.Itoa(x), colorize))
		return err
	case int64:
		_, err := io.WriteString(out, style.Number(strconv.FormatInt(x, 10), colorize))
		return err
	case int32:
		_, err := io.WriteString(out, style.Number(strconv.FormatInt(int64(x), 10), colorize))
		return err
	case uint, uint64, uint32:
		_, err := io.WriteString(out, style.Number(fmt.Sprint(x), colorize))
		return err
	case time.Time:
		_, err := io.WriteString(out, style.Date(x.UTC().Format(time.RFC3339Nano), colorize))
		return err
	case []any:
		if _, err := io.WriteString(out, "["); err != nil {
			return err
		}
		for i, el := range x {
			if i > 0 {
				if _, err := io.WriteString(out, style.Sep(", ", colorize)); err != nil {
					return err
				}
			}
			if err := printValue(out, el, colorize, false); err != nil {
				return err
			}
		}
		_, err := io.WriteString(out, "]")
		return err
	case map[string]any:
		return printMap(out, x, colorize, outside)
	default:
		_, err := io.WriteString(out, fmt.Sprint(x))
		return err
	}
}

func printMap(out io.Writer, m map[string]any, colorize, outside bool) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if !outside {
		if _, err := io.WriteString(out, "{"); err != nil {
			return err
		}
	}
	for i, k := range keys {
		if i > 0 {
			if _, err := io.WriteString(out, style.Sep(", ", colorize)); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(out, style.Key(k, colorize)); err != nil {
			return err
		}
		if _, err := io.WriteString(out, style.Sep(": ", colorize)); err != nil {
			return err
		}
		if err := printValue(out, m[k], colorize, false); err != nil {
			return err
		}
	}
	if !outside {
		_, err := io.WriteString(out, "}")
		return err
	}
	return nil
}
