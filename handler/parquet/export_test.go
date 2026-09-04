package parquet

// WriteParquetFileForTest writes rows using the production schema builder (tests only).
func WriteParquetFileForTest(path string, rows []map[string]any) error {
	seen := make(map[string]struct{}, 16)
	var cols []string
	for _, row := range rows {
		for k := range row {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			cols = append(cols, k)
		}
	}
	return writeParquetFile(path, cols, rows)
}
