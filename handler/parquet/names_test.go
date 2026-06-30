package parquet

import "testing"

func TestCollectionNameStripsArchiveSuffix(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"orders.parquet", "orders"},
		{"orders-0000001.pqt", "orders"},
		{"orders-1.pqt", "orders"},
		{"/data/gcs/orders-0000042.parquet", "orders"},
		{"events", "events"},
	}
	for _, tc := range tests {
		if got := collectionName(tc.path); got != tc.want {
			t.Fatalf("%q => %q want %q", tc.path, got, tc.want)
		}
	}
}

func TestParseColumns(t *testing.T) {
	cols := parseColumns(" id , name ")
	if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Fatalf("cols %+v", cols)
	}
}
