// Package stage defines pipeline stage kinds used during parse and run.
package stage

// Kind classifies a pipeline segment after handler resolution.
type Kind uint8

const (
	Source Kind = iota // emits collections (file, DB query)
	Transform          // maps rows (flatten, coerce)
	Sink               // terminal consumer (file write, DB insert)
)
