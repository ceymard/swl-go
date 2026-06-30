package cli

import (
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// argLexer tokenizes handler argv: flags (--foo, -x) and bare arguments.
var argLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Flag", Pattern: `--?[a-zA-Z][a-zA-Z0-9_-]*`},
	{Name: "Arg", Pattern: `\S+`},
})

// BuildParser constructs a Participle parser for a handler options struct.
func BuildParser[T any]() (*participle.Parser[T], error) {
	return participle.Build[T](
		participle.Lexer(argLexer),
	)
}

// ParseArgs expands flags then parses argv into T using the given parser.
func ParseArgs[T any](parser *participle.Parser[T], argv []string) (*T, error) {
	argv = ExpandFlags(argv)
	return ParseArgsNoExpand(parser, argv)
}

// ParseArgsNoExpand parses argv without ExpandFlags (for pre-split flag tokens).
func ParseArgsNoExpand[T any](parser *participle.Parser[T], argv []string) (*T, error) {
	return parser.ParseString("", formatArgList(argv))
}

func formatArgList(argv []string) string {
	var b strings.Builder
	for i, arg := range argv {
		if i > 0 {
			b.WriteByte(' ')
		}
		if argNeedsQuote(arg) {
			b.WriteByte('"')
			b.WriteString(strings.ReplaceAll(arg, `"`, `\"`))
			b.WriteByte('"')
		} else {
			b.WriteString(arg)
		}
	}
	return b.String()
}

func argNeedsQuote(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		switch r {
		case ' ', ',', ';', '\t':
			return true
		}
	}
	return false
}
