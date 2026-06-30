package parquet

// WriteParquetFileForTest writes rows using the production schema builder (tests only).
func WriteParquetFileForTest(path string, rows []map[string]any) error {
	return writeParquetFile(path, rows)
}
