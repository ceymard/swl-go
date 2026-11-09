package pg_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ceymard/swl-go/handler/pg"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mapCollection(name string, rows ...map[string]any) coll.Collection {
	cs := coll.NewColumnSet()
	out := make([]coll.Row, len(rows))
	for i, r := range rows {
		out[i] = coll.RowFromMap(cs, r)
	}
	return coll.Collection{Name: name, Columns: cs, Rows: coll.SliceRowBatches(out)}
}

func collStream(cols ...coll.Collection) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for _, c := range cols {
			if !yield(c, nil) {
				return
			}
		}
	}
}

func countRows(t *testing.T, ctx context.Context, uri, table string) int {
	t.Helper()
	pool, err := pgxpool.New(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIntegrationSinkValidation(t *testing.T) {
	uri := startPostgres(t)
	ctx := context.Background()
	execSQL(t, ctx, uri, `
		CREATE TABLE val_a (id int PRIMARY KEY, qty float8, label varchar(3) NOT NULL, amount numeric(5,2), mood text);
		CREATE TABLE val_b (id int PRIMARY KEY, name text NOT NULL);`)

	var stderr bytes.Buffer
	cfg := handlersConfig()
	cfg.Messages = &msg.Log{Out: &stderr, Verbose: 0}

	opts, err := pg.ParseSinkOptions(uri, []string{"--ignore-non-existing"})
	if err != nil {
		t.Fatal(err)
	}
	in := collStream(
		mapCollection("val_a",
			map[string]any{"id": 1, "qty": "1.5", "label": "abc", "amount": "123.45"},
			map[string]any{"id": 2, "qty": "invalid", "label": "abc", "amount": "1"},
			map[string]any{"id": 3, "qty": 1, "label": nil, "amount": "1"},
			map[string]any{"id": 1, "qty": 1, "label": "abc", "amount": "1"},
			map[string]any{"id": 4, "qty": 1, "label": "abcd", "amount": "9999.9"},
			map[string]any{"id": 5, "qty": 1, "label": "abc", "amount": "1", "ghost": "boo"},
			map[string]any{"id": 6, "qty": 1, "label": "abc", "amount": "1"},
		),
		// Still validated after the first collection failed.
		mapCollection("val_b",
			map[string]any{"id": 1, "name": "ok"},
			map[string]any{"id": 2, "name": nil},
		),
	)
	err = pg.Sink{}.Sink(ctx, cfg, in, opts)
	if err == nil || !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("want validation error, got %v", err)
	}
	out := stderr.String()
	for _, want := range []string{
		"val_a[2] qty: invalid input syntax for type double precision",
		"warning: val_a[3] label: null value in NOT NULL column",
		"warning: val_a[4] id: duplicate unique key, first seen at row 1",
		"val_a[5] ",
		"label: value too long for type character varying(3)",
		"amount: numeric field overflow",
		"val_a: columns do not exist in the table: ghost",
		"warning: val_b[2] name: null value in NOT NULL column",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "val_a[6]") {
		t.Errorf("unknown columns are reported once per table, not per row:\n%s", out)
	}
	if n := countRows(t, ctx, uri, "val_a"); n != 0 {
		t.Errorf("val_a has %d rows, nothing should have been committed", n)
	}
	if n := countRows(t, ctx, uri, "val_b"); n != 0 {
		t.Errorf("val_b has %d rows, nothing should have been committed", n)
	}

	// The same data, once valid, still loads; --no-check skips validation.
	good := collStream(mapCollection("val_b", map[string]any{"id": 1, "name": "ok"}))
	if err := (pg.Sink{}).Sink(ctx, handlersConfig(), good, opts); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, ctx, uri, "val_b"); n != 1 {
		t.Errorf("val_b has %d rows, want 1", n)
	}

	noCheck, err := pg.ParseSinkOptions(uri, []string{"--no-check"})
	if err != nil {
		t.Fatal(err)
	}
	bad := collStream(mapCollection("val_b", map[string]any{"id": 2, "name": nil}))
	stderr.Reset()
	if err := (pg.Sink{}).Sink(ctx, cfg, bad, noCheck); err == nil {
		t.Fatal("Postgres itself should still reject the null")
	}
	if strings.Contains(stderr.String(), "val_b[1]") {
		t.Errorf("--no-check must not print validation lines:\n%s", stderr.String())
	}
}

func TestParseSinkValidationOptions(t *testing.T) {
	o, err := pg.ParseSinkOptions("postgres://localhost/db", []string{"--no-check", "--max-rejects", "0", "--max-key-memory", "8"})
	if err != nil {
		t.Fatal(err)
	}
	s := o.(pg.SinkOpts)
	if !s.NoCheck || s.MaxRejects != 0 || s.MaxKeyMemMB != 8 {
		t.Fatalf("%+v", s)
	}
	d, _ := pg.ParseSinkOptions("postgres://localhost/db", nil)
	if ds := d.(pg.SinkOpts); ds.NoCheck || ds.MaxRejects != 100 || ds.MaxKeyMemMB != 512 {
		t.Fatalf("defaults %+v", ds)
	}
}

func TestIntegrationSinkValidationTriggerFills(t *testing.T) {
	uri := startPostgres(t)
	ctx := context.Background()
	execSQL(t, ctx, uri, `
		CREATE TABLE trg (id int PRIMARY KEY, name text NOT NULL, code text NOT NULL);
		CREATE FUNCTION trg_fill() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			NEW.name := COALESCE(NEW.name, 'n/a');
			NEW.code := COALESCE(NEW.code, 'auto');
			RETURN NEW;
		END $$;
		CREATE TRIGGER trg_fill BEFORE INSERT ON trg FOR EACH ROW EXECUTE FUNCTION trg_fill();`)

	var stderr bytes.Buffer
	cfg := handlersConfig()
	cfg.Messages = &msg.Log{Out: &stderr, Verbose: 0}
	opts, err := pg.ParseSinkOptions(uri, nil)
	if err != nil {
		t.Fatal(err)
	}
	// "code" is absent and name is null on row 2.
	in := collStream(mapCollection("trg",
		map[string]any{"id": 1, "name": "a"},
		map[string]any{"id": 2, "name": nil},
	))
	if err := (pg.Sink{}).Sink(ctx, cfg, in, opts); err != nil {
		t.Fatalf("a trigger fills these columns, load must not be blocked: %v\n%s", err, stderr.String())
	}
	if n := countRows(t, ctx, uri, "trg WHERE code = 'auto' AND name IN ('a', 'n/a')"); n != 2 {
		t.Errorf("%d rows loaded by the trigger, want 2", n)
	}
	out := stderr.String()
	for _, want := range []string{
		"warning: trg: column code is NOT NULL without default",
		"warning: trg[2] name: null value in NOT NULL column",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr missing %q in:\n%s", want, out)
		}
	}
}
