package csv

import (
	"strconv"
	"strings"

	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/errs"
)

// SrcOpts is parsed argv for csv-src.
type SrcOpts struct {
	Files           []string
	Delimiter       string
	Gunzip          bool
	Quote           rune
	NoEmpty         bool
	EmptyIsNull     bool
	Numbers         bool
	Escape          rune
	Encoding        string
	Headers         []string // non-empty → fixed headers, no header row in file
	Collection      *string
	Merge           map[string]any
	SimplifyHeaders bool
	cli.BaseOpts
}

// SinkOpts is parsed argv for csv-sink.
type SinkOpts struct {
	Path       string
	Delimiter  rune
	Quote      rune
	Charset    string
	NoHeaders  bool
	cli.BaseOpts
}

// ParseSrcOptions parses csv-src argv; target is the first file path.
func ParseSrcOptions(target string, tail []string) (any, error) {
	files, flagTokens, err := splitSrcArgv(target, tail)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errs.New("csv source requires at least one file")
	}
	flags, err := parseSrcFlags(flagTokens)
	if err != nil {
		return nil, err
	}

	opts := SrcOpts{
		Files:           files,
		Delimiter:       ",",
		Quote:           '"',
		Encoding:        "utf-8",
		Gunzip:          flags.Gunzip,
		NoEmpty:         flags.NoEmpty,
		EmptyIsNull:     flags.EmptyIsNull,
		Numbers:         flags.Numbers,
		SimplifyHeaders: flags.SimplifyHeaders,
		Collection:      flags.Collection,
		BaseOpts:        flags.BaseOpts,
	}
	if flags.Delimiter != "" {
		opts.Delimiter = flags.Delimiter
	}
	if flags.Quote != "" {
		opts.Quote = rune(flags.Quote[0])
	}
	if flags.Escape != "" {
		opts.Escape = rune(flags.Escape[0])
	}
	if flags.Encoding != "" {
		opts.Encoding = flags.Encoding
	}
	if flags.Headers != "" {
		opts.Headers = splitList(flags.Headers)
	}
	if flags.Merge != "" {
		opts.Merge = mergeColumns(flags.Merge)
	}
	return opts, nil
}

// ParseSinkOptions parses csv-sink argv; target is the output path.
func ParseSinkOptions(target string, tail []string) (any, error) {
	flags, err := parseSinkFlags(tail)
	if err != nil {
		return nil, err
	}
	opts := SinkOpts{
		Path:      target,
		Delimiter: ';',
		Quote:     '"',
		Charset:   "utf-8",
		NoHeaders: flags.NoHeaders,
		BaseOpts:  flags.BaseOpts,
	}
	if flags.Delimiter != "" {
		opts.Delimiter = rune(flags.Delimiter[0])
	}
	if flags.Quote != "" {
		opts.Quote = rune(flags.Quote[0])
	}
	if flags.Charset != "" {
		opts.Charset = flags.Charset
	}
	return opts, nil
}

type srcFlags struct {
	Delimiter       string
	Gunzip          bool
	Quote           string
	NoEmpty         bool
	EmptyIsNull     bool
	Numbers         bool
	Escape          string
	Encoding        string
	Headers         string
	Collection      *string
	Merge           string
	SimplifyHeaders bool
	cli.BaseOpts
}

type sinkFlags struct {
	Delimiter string
	Quote     string
	Charset   string
	NoHeaders bool
	cli.BaseOpts
}

func parseSrcFlags(tokens []string) (srcFlags, error) {
	var f srcFlags
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok {
		case "--gunzip":
			f.Gunzip = true
		case "-n", "--no-empty":
			f.NoEmpty = true
		case "-N", "--empty-null":
			f.EmptyIsNull = true
		case "-u":
			f.Numbers = true
		case "-s", "--simplify-headers":
			f.SimplifyHeaders = true
		case "-p", "--passthrough":
			f.Passthrough = true
		case "-d", "--delimiter":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Delimiter = v
			i = i2
		case "-q", "--quote":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Quote = v
			i = i2
		case "-e", "--escape":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Escape = v
			i = i2
		case "-E", "--encoding":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Encoding = v
			i = i2
		case "-h", "--headers":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Headers = v
			i = i2
		case "-c", "--collection":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Collection = &v
			i = i2
		case "-m", "--merge":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Merge = v
			i = i2
		case "-a", "--alias":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Alias = &v
			i = i2
		case "-v", "--verbose":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			var n int
			if _, err := strconv.Atoi(v); err == nil {
				n, _ = strconv.Atoi(v)
			}
			f.Verbose = n
			i = i2
		default:
			return f, errs.New("unknown csv source flag: " + tok)
		}
	}
	return f, nil
}

func parseSinkFlags(tokens []string) (sinkFlags, error) {
	tokens = cli.ExpandFlags(tokens)
	var f sinkFlags
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok {
		case "-n", "--no-headers":
			f.NoHeaders = true
		case "-p", "--passthrough":
			f.Passthrough = true
		case "-d", "--delimiter":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Delimiter = v
			i = i2
		case "-q", "--quote":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Quote = v
			i = i2
		case "--charset":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Charset = v
			i = i2
		case "-a", "--alias":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			f.Alias = &v
			i = i2
		case "-v", "--verbose":
			v, i2, err := nextArg(tokens, i)
			if err != nil {
				return f, err
			}
			var n int
			if _, err := strconv.Atoi(v); err == nil {
				n, _ = strconv.Atoi(v)
			}
			f.Verbose = n
			i = i2
		default:
			return f, errs.New("unknown csv sink flag: " + tok)
		}
	}
	return f, nil
}

func nextArg(tokens []string, i int) (string, int, error) {
	if i+1 >= len(tokens) {
		return "", i, errs.New("missing value for flag " + tokens[i])
	}
	return tokens[i+1], i + 1, nil
}

// splitSrcArgv separates file paths from flag tokens.
func splitSrcArgv(target string, tail []string) (files, flags []string, err error) {
	args := cli.ExpandFlags(append([]string{target}, tail...))
	boolFlags := map[string]bool{
		"--gunzip": true, "-n": true, "--no-empty": true,
		"-N": true, "--empty-null": true, "-u": true,
		"-s": true, "--simplify-headers": true,
		"-p": true, "--passthrough": true,
	}
	valueFlags := map[string]bool{
		"-d": true, "--delimiter": true, "-q": true, "--quote": true,
		"-e": true, "--escape": true, "-E": true, "--encoding": true,
		"-h": true, "--headers": true, "-c": true, "--collection": true,
		"-m": true, "--merge": true, "-a": true, "--alias": true,
		"-v": true, "--verbose": true,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			files = append(files, a)
			continue
		}
		flags = append(flags, a)
		if boolFlags[a] {
			continue
		}
		if valueFlags[a] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return files, flags, nil
}
