package handler

import (
	"github.com/ceymard/swl-go/handler/coerce"
	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/handler/csv"
	"github.com/ceymard/swl-go/handler/duckdb"
	"github.com/ceymard/swl-go/handler/json"
	"github.com/ceymard/swl-go/handler/mysql"
	"github.com/ceymard/swl-go/handler/pg"
	"github.com/ceymard/swl-go/handler/parquet"
	"github.com/ceymard/swl-go/handler/sqlite"
	"github.com/ceymard/swl-go/handler/unflatten"
	"github.com/ceymard/swl-go/handler/xlsx"
)

func init() {
	// Implemented transforms (swl2: swl-flatten.ts, swl-coerce.ts, …).
	Register("flatten", flatten.Transform{}, Meta{TransformOnly: true})
	RegisterParser("flatten", func(_ string, _ []string) (any, error) { return flatten.Options{}, nil })

	Register("unflatten", unflatten.Transform{}, Meta{TransformOnly: true})
	RegisterParser("unflatten", func(_ string, tail []string) (any, error) { return unflatten.ParseOptions(tail) })
	RegisterOptParser("unflatten", unflatten.OptParser())

	Register("coerce", coerce.Transform{}, Meta{TransformOnly: true})
	RegisterParser("coerce", func(_ string, tail []string) (any, error) { return coerce.ParseOptions(tail) })
	RegisterOptParser("coerce", coerce.OptParser())

	Register("uncoerce", coerce.UncoerceTransform{}, Meta{TransformOnly: true})
	RegisterParser("uncoerce", func(_ string, tail []string) (any, error) { return coerce.ParseUncoerceOptions(tail) })
	RegisterOptParser("uncoerce", coerce.UncoerceOptParser())

	// JSON source + sink (swl2 swl-json-src/sink.ts).
	Register("json-src", json.Source{}, Meta{})
	RegisterParser("json-src", json.ParseSrcOptions)
	RegisterOptParser("json-src", json.SrcOptParser())
	Register("json-sink", json.Sink{}, Meta{})
	RegisterParser("json-sink", json.ParseSinkOptions)
	RegisterOptParser("json-sink", json.SinkOptParser())

	// SQLite source + sink (swl2 swl-sqlite-src/sink.ts).
	Register("sqlite-src", sqlite.Source{}, Meta{})
	RegisterParser("sqlite-src", sqlite.ParseSrcOptions)
	RegisterOptParser("sqlite-src", sqlite.SrcOptParser())
	Register("sqlite-sink", sqlite.Sink{}, Meta{})
	RegisterParser("sqlite-sink", sqlite.ParseSinkOptions)
	RegisterOptParser("sqlite-sink", sqlite.SinkOptParser())

	// CSV source + sink (swl2 swl-csv-src/sink.ts).
	Register("csv-src", csv.Source{}, Meta{})
	RegisterParser("csv-src", csv.ParseSrcOptions)
	RegisterOptParser("csv-src", csv.SrcOptParser())
	Register("csv-sink", csv.Sink{}, Meta{})
	RegisterParser("csv-sink", csv.ParseSinkOptions)
	RegisterOptParser("csv-sink", csv.SinkOptParser())

	// PostgreSQL source + sink (swl2 swl-pg-src/sink.ts).
	Register("pg-src", pg.Source{}, Meta{})
	RegisterParser("pg-src", pg.ParseSrcOptions)
	RegisterOptParser("pg-src", pg.SrcOptParser())
	Register("pg-sink", pg.Sink{}, Meta{})
	RegisterParser("pg-sink", pg.ParseSinkOptions)
	RegisterOptParser("pg-sink", pg.SinkOptParser())

	// Spreadsheet source (swl2 swl-xlsx-src.ts): xlsx, xlsb, xlsm, ods.
	Register("xlsx-src", xlsx.Source{}, Meta{})
	RegisterParser("xlsx-src", xlsx.ParseSrcOptions)
	RegisterOptParser("xlsx-src", xlsx.SrcOptParser())
	Register("xlsx-sink", xlsx.Sink{}, Meta{})
	RegisterParser("xlsx-sink", xlsx.ParseSinkOptions)
	RegisterOptParser("xlsx-sink", xlsx.SinkOptParser())

	// Parquet source + sink (swl2 swl-parquet-src/sink.ts).
	Register("parquet-src", parquet.Source{}, Meta{})
	RegisterParser("parquet-src", parquet.ParseSrcOptions)
	RegisterOptParser("parquet-src", parquet.SrcOptParser())
	Register("parquet-sink", parquet.Sink{}, Meta{})
	RegisterParser("parquet-sink", parquet.ParseSinkOptions)
	RegisterOptParser("parquet-sink", parquet.SinkOptParser())

	// DuckDB source + sink (swl2 swl-duckdb-src/sink.ts).
	Register("duckdb-src", duckdb.Source{}, Meta{})
	RegisterParser("duckdb-src", duckdb.ParseSrcOptions)
	RegisterOptParser("duckdb-src", duckdb.SrcOptParser())
	Register("duckdb-sink", duckdb.Sink{}, Meta{})
	RegisterParser("duckdb-sink", duckdb.ParseSinkOptions)
	RegisterOptParser("duckdb-sink", duckdb.SinkOptParser())

	// MySQL source + sink (swl2 swl-my-src.ts; sink is swl-go extension).
	Register("my-src", mysql.Source{}, Meta{})
	RegisterParser("my-src", mysql.ParseSrcOptions)
	RegisterOptParser("my-src", mysql.SrcOptParser())
	Register("my-sink", mysql.Sink{}, Meta{})
	RegisterParser("my-sink", mysql.ParseSinkOptions)
	RegisterOptParser("my-sink", mysql.SinkOptParser())

	registerStubs()
}

// registerStubs wires placeholder handlers for not-yet-ported swl2 scripts.
func registerStubs() {
	stubs := []string{
		"yaml-src", "yaml-sink",
		"fn",
	}
	for _, id := range stubs {
		Register(id, stubHandler{id: id}, Meta{Stub: true})
	}
}
