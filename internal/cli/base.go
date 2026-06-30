// Package cli tokenizes argv and parses handler flags with Participle v2.
//
// Each handler embeds BaseOpts for shared flags; handler-specific grammars live
// in handler/* packages. We do not port swl2 optparse.ts — grammars are per-handler.
package cli

// BaseOpts are shared swl handler flags (-p, -a, -v).
// Tag values must use parser:"..." (valid reflect.StructTag) — see participle docs.
type BaseOpts struct {
	Passthrough bool    `parser:"( '-p' | '--passthrough' )?"` // tee rows to stderr while processing
	Alias       *string `parser:"( ( '-a' | '--alias' ) @Arg )?"` // rename collection/table
	Verbose     int     `parser:"( ( '-v' | '--verbose' ) @Arg )?"` // handler-local verbosity
}
