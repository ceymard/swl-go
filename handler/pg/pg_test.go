package pg_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/pg"
)

func TestParseSrcOptions(t *testing.T) {
	opts, err := pg.ParseSrcOptions("postgres://localhost/db", []string{"-s", "app", "users", "-q", "select 1"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(pg.SrcOpts)
	if o.URI != "postgres://localhost/db" || o.Schema != "app" {
		t.Fatalf("%+v", o)
	}
	if len(o.Sources) != 1 || o.Sources[0].Name != "users" || o.Sources[0].Query != "select 1" {
		t.Fatalf("%+v", o.Sources)
	}
}

func TestParseSinkOptions(t *testing.T) {
	opts, err := pg.ParseSinkOptions("postgres://localhost/db", []string{"--auto-create", "-u", "users"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(pg.SinkOpts)
	if !o.AutoCreate || !o.Upsert {
		t.Fatalf("%+v", o)
	}
	c, ok := o.Collections["users"]
	if !ok || !c.Upsert {
		t.Fatalf("collections %+v", o.Collections)
	}
}
