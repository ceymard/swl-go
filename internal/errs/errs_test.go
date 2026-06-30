package errs_test

import (
	"testing"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/samber/oops"
)

func TestWrapStacktrace(t *testing.T) {
	err := errs.Wrap(errs.New("root"), "outer", "k", "v")
	if _, ok := oops.AsOops(err); !ok {
		t.Fatal("expected oops error")
	}
	if st := errs.Stacktrace(err); st == "" {
		t.Fatal("expected stacktrace")
	}
}
