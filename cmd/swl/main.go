// Command swl is the swl-go CLI — a Go port of swl2 for data pipeline ETL.
//
// Pipeline syntax: sources [++ sources] [:: transforms] [:: sink]
// Example: swl users.json ++ orders.csv :: flatten :: app.db
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"
	_ "github.com/ceymard/swl-go/handler" // side-effect: register all handlers
	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/runner"
	"github.com/ceymard/swl-go/internal/style"
)

// globalCLI holds flags parsed by Kong before pipeline tokens.
type globalCLI struct {
	Quiet    bool     `help:"Suppress progress output"`
	Verbose  int      `short:"v" type:"counter" help:"Increase verbosity"`
	Help     bool     `short:"h" help:"Show help"`
	Pipeline []string `arg:"" optional:"" passthrough:"all"` // everything after flags
}

func main() {
	// Match swl2: timestamps are always UTC in logs and coerced dates.
	time.Local = time.UTC

	var cli globalCLI
	ctx := kong.Parse(&cli)

	if cli.Help {
		_ = ctx.PrintUsage(false)
		os.Exit(0)
	}

	// swl2: empty pipeline prints available handlers/extensions/protocols.
	if len(cli.Pipeline) == 0 {
		c := style.Enabled(os.Stderr)
		fmt.Fprintln(os.Stderr, style.Error("error:", c), "a command may not be empty")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, style.Dim("  list of available sources/sinks :", c))
		fmt.Fprintln(os.Stderr)
		handler.WriteAvailable(os.Stdout)
		os.Exit(1)
	}

	// Default verbosity 2 (progress); -q → 0; -v/-vv overrides; SWL_VERBOSE env fallback.
	verbose := 2
	if cli.Quiet {
		verbose = 0
	} else if cli.Verbose > 0 {
		verbose = cli.Verbose
	} else if v := os.Getenv("SWL_VERBOSE"); v != "" {
		fmt.Sscanf(v, "%d", &verbose)
	}

	p, err := pipeline.Parse(cli.Pipeline, verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if st := errs.Stacktrace(err); st != "" && verbose >= 1 {
			fmt.Fprintln(os.Stderr, st)
		}
		os.Exit(1)
	}

	cfg := runner.Config{
		Messages: msg.New(verbose),
		Verbose:  verbose,
	}

	if err := runner.Run(cfg, handler.Reg, p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if st := errs.Stacktrace(err); st != "" && verbose >= 1 {
			fmt.Fprintln(os.Stderr, st)
		}
		os.Exit(1)
	}
}
