// Package cli holds shared handler CLI types used with internal/optparse.
package cli

import "github.com/ceymard/swl-go/internal/optparse"

// BaseOpts are shared swl handler flags (-p, -a, -v).
type BaseOpts struct {
	Passthrough bool
	Alias       *string
	Verbose     int
}

// BaseOptsFrom extracts shared flags from an optparse result map.
func BaseOptsFrom(m map[string]any) BaseOpts {
	var bo BaseOpts
	if v, ok := m["passthrough"].(bool); ok {
		bo.Passthrough = v
	}
	if s, ok := m["alias"].(string); ok && s != "" {
		bo.Alias = &s
	}
	bo.Verbose = optparse.Int(m, "verbose")
	return bo
}
