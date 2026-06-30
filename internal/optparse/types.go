package optparse

// MatchError stops matching for a sub-parser (swl2 MatchError).
type MatchError struct {
	Message string
}

func (e *MatchError) Error() string { return e.Message }

func matchErr(msg string) error { return &MatchError{Message: msg} }

// Handler consumes argv tokens and produces a typed value (swl2 Handler).
type Handler struct {
	Key        string
	Activators []string
	help       string
	group      string
	Repeating  bool
	required   bool
	DefaultVal any
	HasDefault bool
	Bases      []*Parser

	scan  func(args []string, pos int, acc [][]string) ([]string, error)
	value func(groups [][]string, ctx *parseCtx) (any, error)
}

// Parser is a CLI option parser (swl2 CliParser).
type Parser struct {
	handlers []*Handler
}

type parseCtx struct {
	oneof map[string]oneofEntry
}

type oneofEntry struct {
	mapres map[*Handler][][]string
	parser *Parser
}

type scanResult struct {
	pos    int
	mapres map[*Handler][][]string
}
