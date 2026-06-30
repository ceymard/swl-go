package handler

import (
	"github.com/ceymard/swl-go/handler/coerce"
	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/handler/json"
	"github.com/ceymard/swl-go/handler/sqlite"
	"github.com/ceymard/swl-go/handler/unflatten"
)

func init() {
	// Implemented transforms (swl2: swl-flatten.ts, swl-coerce.ts, …).
	Register("flatten", flatten.Transform{}, Meta{TransformOnly: true})
	RegisterParser("flatten", func(_ string, _ []string) (any, error) { return flatten.Options{}, nil })

	Register("unflatten", unflatten.Transform{}, Meta{TransformOnly: true})
	RegisterParser("unflatten", func(_ string, tail []string) (any, error) { return unflatten.ParseOptions(tail) })

	Register("coerce", coerce.Transform{}, Meta{TransformOnly: true})
	RegisterParser("coerce", func(_ string, tail []string) (any, error) { return coerce.ParseOptions(tail) })

	Register("uncoerce", coerce.UncoerceTransform{}, Meta{TransformOnly: true})
	RegisterParser("uncoerce", func(_ string, tail []string) (any, error) { return coerce.ParseUncoerceOptions(tail) })

	// JSON source + sink (swl2 swl-json-src/sink.ts).
	Register("json-src", json.Source{}, Meta{})
	RegisterParser("json-src", json.ParseSrcOptions)
	Register("json-sink", json.Sink{}, Meta{})
	RegisterParser("json-sink", json.ParseSinkOptions)

	// SQLite source + sink (swl2 swl-sqlite-src/sink.ts).
	Register("sqlite-src", sqlite.Source{}, Meta{})
	RegisterParser("sqlite-src", sqlite.ParseSrcOptions)
	Register("sqlite-sink", sqlite.Sink{}, Meta{})
	RegisterParser("sqlite-sink", sqlite.ParseSinkOptions)

	registerStubs()
}

// registerStubs wires placeholder handlers for not-yet-ported swl2 scripts.
func registerStubs() {
	stubs := []string{
		"csv-src", "csv-sink",
		"pg-src", "pg-sink", "my-src",
		"duckdb-src", "duckdb-sink",
		"xlsx-src", "xlsx-sink", "yaml-src", "yaml-sink",
		"parquet-src", "parquet-sink", "fn",
	}
	for _, id := range stubs {
		Register(id, stubHandler{id: id}, Meta{Stub: true})
	}
}
