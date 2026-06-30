package mssql_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/mssql"
)

func TestParseSrcOptions(t *testing.T) {
	opts, err := mssql.ParseSrcOptions("sqlserver://swl:SwlPassw0rd!@localhost/swltest", []string{"dbo.users", "-q", "select 1"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(mssql.SrcOpts)
	if o.URI != "sqlserver://swl:SwlPassw0rd!@localhost/swltest" {
		t.Fatalf("uri %q", o.URI)
	}
	if len(o.Sources) != 1 || o.Sources[0].Name != "dbo.users" || o.Sources[0].Query != "select 1" {
		t.Fatalf("%+v", o.Sources)
	}
}

func TestParseSinkOptions(t *testing.T) {
	opts, err := mssql.ParseSinkOptions("mssql://swl:SwlPassw0rd!@localhost/swltest", []string{"--auto-create", "-u", "dbo.users"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(mssql.SinkOpts)
	if !o.AutoCreate || !o.Upsert {
		t.Fatalf("%+v", o)
	}
}
