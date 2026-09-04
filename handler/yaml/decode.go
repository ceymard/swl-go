package yaml

import (
	"strconv"
	"strings"
	"time"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/dop251/goja"
	yamlv3 "gopkg.in/yaml.v3"
)

type parsedDoc struct {
	keys []string
	data map[string][]any
}

type jsGenerator struct {
	rt *evalRuntime
	fn goja.Callable
}

func parseDocument(data []byte, defaultCollection string, rt *evalRuntime) (*parsedDoc, error) {
	var root yamlv3.Node
	if err := yamlv3.Unmarshal(data, &root); err != nil {
		return nil, errs.Wrap(err, "parse yaml")
	}
	if len(root.Content) == 0 {
		return &parsedDoc{data: map[string][]any{}}, nil
	}
	doc := root.Content[0]
	switch doc.Kind {
	case yamlv3.MappingNode:
		return decodeMappingDoc(doc, rt)
	case yamlv3.SequenceNode:
		items, err := decodeSequence(doc, rt)
		if err != nil {
			return nil, err
		}
		name := defaultCollection
		if name == "" {
			name = "yaml"
		}
		return &parsedDoc{
			keys: []string{name},
			data: map[string][]any{name: items},
		}, nil
	default:
		return nil, errs.New("yaml root must be a mapping or sequence")
	}
}

func decodeMappingDoc(node *yamlv3.Node, rt *evalRuntime) (*parsedDoc, error) {
	out := &parsedDoc{data: make(map[string][]any)}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		name, err := scalarString(keyNode)
		if err != nil {
			return nil, err
		}
		items, err := decodeCollectionItems(valNode, rt)
		if err != nil {
			return nil, err
		}
		out.keys = append(out.keys, name)
		out.data[name] = items
	}
	return out, nil
}

func decodeCollectionItems(node *yamlv3.Node, rt *evalRuntime) ([]any, error) {
	if node.Kind != yamlv3.SequenceNode {
		return nil, errs.New("yaml collection must be a sequence: " + nodeKind(node))
	}
	items := make([]any, 0, len(node.Content))
	for _, child := range node.Content {
		v, err := decodeValue(child, rt)
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, nil
}

func decodeValue(node *yamlv3.Node, rt *evalRuntime) (any, error) {
	if isEvalTag(node.Tag) {
		code, err := evalSource(node)
		if err != nil {
			return nil, err
		}
		val, err := rt.eval(code)
		if err != nil {
			return nil, err
		}
		if fn, ok := valueToCallable(val); ok {
			return &jsGenerator{rt: rt, fn: fn}, nil
		}
		return exportValue(val)
	}
	switch node.Kind {
	case yamlv3.MappingNode:
		return decodeMapping(node, rt)
	case yamlv3.SequenceNode:
		return decodeSequence(node, rt)
	case yamlv3.ScalarNode:
		return decodeScalar(node)
	case yamlv3.AliasNode:
		return nil, errs.New("yaml aliases are not supported")
	default:
		return nil, errs.New("unsupported yaml node: " + nodeKind(node))
	}
}

func evalSource(node *yamlv3.Node) (string, error) {
	switch node.Kind {
	case yamlv3.ScalarNode:
		return node.Value, nil
	default:
		return "", errs.New("yaml eval tag must be a scalar")
	}
}

func decodeMapping(node *yamlv3.Node, rt *evalRuntime) (map[string]any, error) {
	out := make(map[string]any, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, err := scalarString(node.Content[i])
		if err != nil {
			return nil, err
		}
		val, err := decodeValue(node.Content[i+1], rt)
		if err != nil {
			return nil, err
		}
		out[key] = val
	}
	return out, nil
}

func decodeSequence(node *yamlv3.Node, rt *evalRuntime) ([]any, error) {
	out := make([]any, 0, len(node.Content))
	for _, child := range node.Content {
		v, err := decodeValue(child, rt)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func decodeScalar(node *yamlv3.Node) (any, error) {
	switch node.Tag {
	case "!!str", "!!string", "":
		return node.Value, nil
	case "!!int":
		n, err := strconv.ParseInt(node.Value, 10, 64)
		if err != nil {
			return nil, errs.Wrap(err, "parse yaml int")
		}
		return n, nil
	case "!!float":
		n, err := strconv.ParseFloat(node.Value, 64)
		if err != nil {
			return nil, errs.Wrap(err, "parse yaml float")
		}
		return n, nil
	case "!!bool":
		switch strings.ToLower(node.Value) {
		case "true", "y", "yes", "on":
			return true, nil
		case "false", "n", "no", "off":
			return false, nil
		default:
			return nil, errs.New("parse yaml bool: " + node.Value)
		}
	case "!!null", "!!nil":
		return nil, nil
	case "!!timestamp":
		t, err := time.Parse(time.RFC3339Nano, node.Value)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05Z07:00", node.Value)
			if err != nil {
				return node.Value, nil
			}
		}
		return t.UTC(), nil
	default:
		if node.Style == yamlv3.LiteralStyle || node.Style == yamlv3.FoldedStyle {
			return node.Value, nil
		}
		var v any
		if err := node.Decode(&v); err != nil {
			return node.Value, nil
		}
		return v, nil
	}
}

func scalarString(node *yamlv3.Node) (string, error) {
	if node.Kind != yamlv3.ScalarNode {
		return "", errs.New("yaml key must be a scalar: " + nodeKind(node))
	}
	return node.Value, nil
}

func nodeKind(node *yamlv3.Node) string {
	switch node.Kind {
	case yamlv3.DocumentNode:
		return "document"
	case yamlv3.SequenceNode:
		return "sequence"
	case yamlv3.MappingNode:
		return "mapping"
	case yamlv3.ScalarNode:
		return "scalar"
	case yamlv3.AliasNode:
		return "alias"
	default:
		return "unknown"
	}
}

func exportValue(v goja.Value) (any, error) {
	return v.Export(), nil
}

func (g *jsGenerator) run(acc map[string][]any, emit func(any) error) error {
	accVal, err := gojaAccToValue(g.rt.vm, acc)
	if err != nil {
		return err
	}
	push := func(call goja.FunctionCall) goja.Value {
		obj := call.Argument(0).Export()
		if _, ok := obj.(map[string]any); !ok {
			return goja.Undefined()
		}
		_ = emit(obj)
		return goja.Undefined()
	}
	if _, err := g.fn(goja.Undefined(), accVal, g.rt.vm.ToValue(push)); err != nil {
		return errs.Wrap(err, "yaml generator function")
	}
	return nil
}

func gojaAccToValue(vm *goja.Runtime, acc map[string][]any) (goja.Value, error) {
	jsAcc := vm.NewObject()
	for name, items := range acc {
		arr := vm.NewArray(len(items))
		for i, item := range items {
			if err := arr.Set(strconv.Itoa(i), vm.ToValue(item)); err != nil {
				return nil, err
			}
		}
		if err := jsAcc.Set(name, arr); err != nil {
			return nil, err
		}
	}
	return jsAcc, nil
}

// itemToMap reports whether v is an object-shaped YAML value (destined to
// become a positional row via coll.RowFromMap), as opposed to a scalar
// wrapped under a synthetic "value" column.
func itemToMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func stripMeta(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	if _, ok := m["__meta__"]; !ok {
		return m
	}
	out := make(map[string]any, len(m)-1)
	for k, v := range m {
		if k == "__meta__" {
			continue
		}
		out[k] = v
	}
	return out
}
