package yaml

import (
	"strings"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/dop251/goja"
)

// evalRuntime holds shared JS state for !!e tags in one YAML file (swl2 vm context).
type evalRuntime struct {
	vm *goja.Runtime
}

func newEvalRuntime() *evalRuntime {
	return &evalRuntime{vm: goja.New()}
}

func (r *evalRuntime) eval(code string) (goja.Value, error) {
	code = strings.TrimSpace(code)
	// Function declarations must be wrapped to evaluate to a callable value.
	if strings.HasPrefix(code, "function") {
		code = "(" + code + ")"
	}
	val, err := r.vm.RunString(code)
	if err != nil {
		return nil, errs.Wrap(err, "yaml eval tag")
	}
	return val, nil
}

func isEvalTag(tag string) bool {
	switch tag {
	case "!!e", "!e":
		return true
	default:
		return strings.HasSuffix(tag, ":e")
	}
}

func valueToCallable(val goja.Value) (goja.Callable, bool) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, false
	}
	return goja.AssertFunction(val)
}
