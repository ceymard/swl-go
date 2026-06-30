// Package style provides ANSI colors for CLI output (swl2 src/debug.ts palette).
//
// Colors are disabled when NO_COLOR is set or the target stream is not a terminal.
package style

import (
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// Enabled reports whether w should receive ANSI color codes.
func Enabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

// --- swl2 debug.ts roles ---

func Collection(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiYellow).Sprint(s)
}

func LineNum(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiGreen).Sprint(s)
}

func Key(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgCyan).Sprint(s)
}

func Sep(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiBlack, color.Bold).Sprint(s)
}

func String(s string, on bool) string {
	if !on {
		return s
	}
	// hsl(80, 60, 60) — lime-ish string literals
	return color.New(color.FgGreen).Sprint(s)
}

func Number(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiGreen).Sprint(s)
}

func Bool(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiMagenta).Sprint(s)
}

func Null(on bool) string {
	if !on {
		return "null"
	}
	return color.New(color.FgHiRed).Sprint("null")
}

func Date(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiCyan).Sprint(s)
}

func Error(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiRed, color.Bold).Sprint(s)
}

func Dim(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiBlack).Sprint(s)
}

func Heading(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.Bold).Sprint(s)
}

// --- swl2 swl.ts handler sigils ---

func SigilBoth(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiMagenta).Sprint(s)
}

func SigilSource(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiGreen).Sprint(s)
}

func SigilSink(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgHiRed).Sprint(s)
}

func HandlerName(s string, on bool) string {
	if !on {
		return s
	}
	return color.New(color.FgWhite).Sprint(s)
}
