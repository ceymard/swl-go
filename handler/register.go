package handler

import (
	"github.com/ceymard/swl-go/handler/coerce"
	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/handler/unflatten"
)

func init() {
	// Implemented transforms (swl2: swl-flatten.ts, swl-coerce.ts, …).
	Register("flatten", flatten.Transform{}, Meta{TransformOnly: true})
	RegisterParser("flatten", func(argv []string) (any, error) { return flatten.Options{}, nil })

	Register("unflatten", unflatten.Transform{}, Meta{TransformOnly: true})
	RegisterParser("unflatten", unflatten.ParseOptions)

	Register("coerce", coerce.Transform{}, Meta{TransformOnly: true})
	RegisterParser("coerce", coerce.ParseOptions)

	Register("uncoerce", coerce.UncoerceTransform{}, Meta{TransformOnly: true})
	RegisterParser("uncoerce", coerce.ParseUncoerceOptions)

	registerStubs()
}

// registerStubs wires placeholder handlers for not-yet-ported swl2 scripts.
func registerStubs() {
	stubs := []string{
		"json-src", "json-sink", "csv-src", "csv-sink",
		"sqlite-src", "sqlite-sink",
		"pg-src", "pg-sink", "my-src",
		"duckdb-src", "duckdb-sink",
		"xlsx-src", "xlsx-sink", "yaml-src", "yaml-sink",
		"parquet-src", "parquet-sink", "fn",
	}
	for _, id := range stubs {
		Register(id, stubHandler{id: id}, Meta{Stub: true})
	}
}
