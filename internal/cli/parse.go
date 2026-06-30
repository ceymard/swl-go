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
		participle.Unquote("String"),
	)
}

// ParseArgs expands flags then parses argv into T using the given parser.
func ParseArgs[T any](parser *participle.Parser[T], argv []string) (*T, error) {
	argv = ExpandFlags(argv)
	input := strings.Join(argv, " ")
	return parser.ParseString("", input)
}
