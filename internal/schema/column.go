// Package schema holds optional column metadata on collections.
//
// Sources that know types upfront (sqlite DESCRIBE, csv headers) populate Columns;
// others leave it nil and sinks infer from the first row.
package schema

// Column describes one named column with optional SQL-ish type hints.
type Column struct {
	ColumnName string
	ColumnType string // e.g. "TEXT", "INTEGER" — handler-specific interpretation
	NotNull    bool
}
