package pg

import (
	"strings"
	"testing"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCheckers(t *testing.T) {
	numeric := func(p, s int) int32 { return int32(p<<16|s) + 4 }
	cases := []struct {
		name   string
		col    pgColumn
		value  any
		reject bool
	}{
		{"float ok", pgColumn{TypeOID: pgtype.Float8OID}, "1.5e3", false},
		{"float NaN", pgColumn{TypeOID: pgtype.Float8OID}, "NaN", false},
		{"float inf", pgColumn{TypeOID: pgtype.Float8OID}, "-Infinity", false},
		{"float padded", pgColumn{TypeOID: pgtype.Float8OID}, " 2 ", false},
		{"float syntax", pgColumn{TypeOID: pgtype.Float8OID}, "abc", true},
		{"float empty", pgColumn{TypeOID: pgtype.Float8OID}, "", true},
		{"float4 range", pgColumn{TypeOID: pgtype.Float4OID}, "1e50", true},
		{"int ok", pgColumn{TypeOID: pgtype.Int4OID}, float64(3), false},
		{"int padded", pgColumn{TypeOID: pgtype.Int4OID}, " 12 ", false},
		{"int plus", pgColumn{TypeOID: pgtype.Int4OID}, "+12", false},
		{"int fraction", pgColumn{TypeOID: pgtype.Int4OID}, 3.5, true},
		{"int 1e3", pgColumn{TypeOID: pgtype.Int4OID}, "1e3", true},
		{"int range", pgColumn{TypeOID: pgtype.Int2OID}, int64(70000), true},
		{"int underscore (pg16)", pgColumn{TypeOID: pgtype.Int4OID}, "1_000", false},
		{"int hex (pg16)", pgColumn{TypeOID: pgtype.Int4OID}, "0xff", false},
		{"numeric ok", pgColumn{TypeOID: pgtype.NumericOID, Typmod: numeric(5, 2)}, "123.45", false},
		{"numeric exp", pgColumn{TypeOID: pgtype.NumericOID}, "1e5", false},
		{"numeric overflow", pgColumn{TypeOID: pgtype.NumericOID, Typmod: numeric(5, 2)}, "1234.5", true},
		{"numeric round carry", pgColumn{TypeOID: pgtype.NumericOID, Typmod: numeric(3, 2)}, "9.995", true},
		{"numeric rounds fine", pgColumn{TypeOID: pgtype.NumericOID, Typmod: numeric(3, 2)}, "9.994", false},
		{"numeric tiny", pgColumn{TypeOID: pgtype.NumericOID, Typmod: numeric(3, 2)}, "0.001", false},
		{"numeric syntax", pgColumn{TypeOID: pgtype.NumericOID}, "1,5", true},
		{"bool yes", pgColumn{TypeOID: pgtype.BoolOID}, "Yes", false},
		{"bool prefix", pgColumn{TypeOID: pgtype.BoolOID}, "tr", false},
		{"bool native", pgColumn{TypeOID: pgtype.BoolOID}, true, false},
		{"bool bad", pgColumn{TypeOID: pgtype.BoolOID}, "maybe", true},
		{"bool o", pgColumn{TypeOID: pgtype.BoolOID}, "o", true},
		{"uuid ok", pgColumn{TypeOID: pgtype.UUIDOID}, "{123e4567-e89b-12d3-a456-426614174000}", false},
		{"uuid bad", pgColumn{TypeOID: pgtype.UUIDOID}, "123e4567", true},
		{"date ok", pgColumn{TypeOID: pgtype.DateOID}, "2024-02-29", false},
		{"date bad day", pgColumn{TypeOID: pgtype.DateOID}, "2023-02-29", true},
		{"date bad month", pgColumn{TypeOID: pgtype.DateOID}, "2023-13-01", true},
		{"date other spelling", pgColumn{TypeOID: pgtype.DateOID}, "today", false},
		{"date compact", pgColumn{TypeOID: pgtype.DateOID}, "20200102", false},
		{"date empty", pgColumn{TypeOID: pgtype.DateOID}, "", true},
		{"timestamp ok", pgColumn{TypeOID: pgtype.TimestamptzOID}, "2024-01-02T03:04:05.123Z", false},
		{"timestamp bad hour", pgColumn{TypeOID: pgtype.TimestampOID}, "2024-01-02 25:00:00", true},
		{"time bad minute", pgColumn{TypeOID: pgtype.TimeOID}, "10:61:00", true},
		{"varchar ok", pgColumn{TypeOID: pgtype.VarcharOID, Typmod: 9}, "héllo", false},
		{"varchar trailing spaces", pgColumn{TypeOID: pgtype.VarcharOID, Typmod: 9}, "hello     ", false},
		{"varchar too long", pgColumn{TypeOID: pgtype.VarcharOID, Typmod: 9}, "hello world", true},
		{"text nul", pgColumn{TypeOID: pgtype.TextOID}, "a\x00b", true},
		{"enum ok", pgColumn{TypeOID: 99999, TypeName: "mood", EnumVals: []string{"sad", "ok"}}, "ok", false},
		{"enum bad", pgColumn{TypeOID: 99999, TypeName: "mood", EnumVals: []string{"sad", "ok"}}, "OK", true},
		{"json anything", pgColumn{TypeOID: pgtype.JSONBOID}, "not json", false},
	}
	for _, tc := range cases {
		chk := checkerFor(tc.col)
		var e *cellErr
		if chk != nil {
			e = chk(cellText(tc.value))
		}
		if (e != nil) != tc.reject {
			t.Errorf("%s: value %#v reject=%v, want %v (%v)", tc.name, tc.value, e != nil, tc.reject, e)
		}
	}
}

func newTestValidator(t *testing.T, meta *pgTableMeta, cols []string, o validatorOpts) (*tableValidator, *[]string) {
	t.Helper()
	var lines []string
	o.emit = func(s string) { lines = append(lines, s) }
	cs := coll.NewColumnSet()
	for _, c := range cols {
		cs.Index(c)
	}
	return newTableValidator("t", meta, cols, cs, o), &lines
}

func TestValidatorRows(t *testing.T) {
	meta := &pgTableMeta{
		Columns: []pgColumn{
			{Name: "id", TypeOID: pgtype.Int4OID, NotNull: true},
			{Name: "qty", TypeOID: pgtype.Float8OID},
			{Name: "created", TypeOID: pgtype.DateOID, NotNull: true, HasDef: true},
			{Name: "req", TypeOID: pgtype.TextOID, NotNull: true},
		},
		Uniques: []pgUnique{{Columns: []string{"id"}, IsPrimary: true}},
	}
	v, lines := newTestValidator(t, meta, []string{"id", "qty", "created", "extra"}, validatorOpts{
		maxRejects: 2, insertMode: true, checkDups: true,
	})
	if len(v.tableErrs) != 1 || !strings.Contains(v.tableErrs[0], "req") {
		t.Fatalf("table errors: %v", v.tableErrs)
	}
	rows := []coll.Row{
		{1, "1.5", "2024-01-01", nil},
		{2, "abc", nil, nil},           // bad float, explicit null overrides default
		{1, 2.0, "2024-01-01", nil},    // duplicate id
		{3, 1.0, "2024-01-01", "oops"}, // unknown column holds a value
		{nil, 1.0, "2024-01-01", nil},  // null id
		{4, 1.0, "2024-01-01", nil, 7}, // column discovered after open
	}
	var ok []bool
	for i, r := range rows {
		ok = append(ok, v.check(r, int64(i+1)))
	}
	want := []bool{true, false, false, false, false, false}
	for i := range want {
		if ok[i] != want[i] {
			t.Errorf("row %d: ok=%v want %v", i+1, ok[i], want[i])
		}
	}
	if len(*lines) != 2 {
		t.Fatalf("detail lines capped at 2, got %d: %q", len(*lines), *lines)
	}
	if !strings.Contains((*lines)[0], "t[2] qty: invalid input syntax for type double precision") ||
		!strings.Contains((*lines)[0], "created: null value in NOT NULL column") {
		t.Errorf("row 2 line: %s", (*lines)[0])
	}
	v.summary()
	all := strings.Join(*lines, "\n")
	if !strings.Contains(all, "t: 5 invalid rows") || !strings.Contains(all, "id: duplicate key: 1") {
		t.Errorf("summary missing:\n%s", all)
	}
}

func TestValidatorDupSkippedWithNulls(t *testing.T) {
	meta := &pgTableMeta{
		Columns: []pgColumn{{Name: "a", TypeOID: pgtype.Int4OID}},
		Uniques: []pgUnique{{Columns: []string{"a"}}},
	}
	v, _ := newTestValidator(t, meta, []string{"a"}, validatorOpts{insertMode: true, checkDups: true})
	for i := 0; i < 3; i++ {
		if !v.check(coll.Row{nil}, int64(i+1)) {
			t.Fatal("NULLs never conflict in a unique index")
		}
	}
}

func TestValidatorKeyMemoryLimit(t *testing.T) {
	meta := &pgTableMeta{
		Columns: []pgColumn{{Name: "a", TypeOID: pgtype.Int4OID}},
		Uniques: []pgUnique{{Columns: []string{"a"}}},
	}
	v, lines := newTestValidator(t, meta, []string{"a"}, validatorOpts{insertMode: true, checkDups: true, keyLimit: 200})
	for i := 0; i < 10; i++ {
		v.check(coll.Row{i}, int64(i+1))
	}
	if !v.keysOff || len(*lines) != 1 {
		t.Fatalf("keysOff=%v lines=%q", v.keysOff, *lines)
	}
	if !v.check(coll.Row{0}, 11) {
		t.Fatal("duplicates are no longer tracked once the limit is hit")
	}
}
