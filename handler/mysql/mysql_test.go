package mysql_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/mysql"
)

func TestParseSrcOptions(t *testing.T) {
	opts, err := mysql.ParseSrcOptions("mysql://swl:swl@localhost/swltest", []string{"users", "-q", "select 1"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(mysql.SrcOpts)
	if o.URI != "mysql://swl:swl@localhost/swltest" {
		t.Fatalf("uri %q", o.URI)
	}
	if len(o.Sources) != 1 || o.Sources[0].Name != "users" || o.Sources[0].Query != "select 1" {
		t.Fatalf("%+v", o.Sources)
	}
}

func TestParseSinkOptions(t *testing.T) {
	opts, err := mysql.ParseSinkOptions("mysql://swl:swl@localhost/swltest", []string{"--auto-create", "-u", "users"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(mysql.SinkOpts)
	if !o.AutoCreate || !o.Upsert {
		t.Fatalf("%+v", o)
	}
}
