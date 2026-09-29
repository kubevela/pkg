/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cel

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"

	"github.com/kubevela/pkg/cel/template"
)

// OutputType compiles an expression and reports the type it produces, from the
// AST alone, before any data exists.
func (e *Engine) OutputType(env *cel.Env, expr string) (*cel.Type, error) {
	if env == e.dyn {
		c, err := e.compile(env, expr)
		if err != nil {
			return nil, err
		}
		return c.ast.OutputType(), nil
	}
	e.checkedMu.Lock()
	cache := e.checked[env]
	e.checkedMu.Unlock()
	if cache != nil {
		if hit, ok := cache.Get(expr); ok {
			if ast, ok := hit.(*cel.Ast); ok {
				return ast.OutputType(), nil
			}
		}
	}
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, iss.Err()
	}
	if cache != nil {
		cache.Add(expr, ast)
	}
	return ast.OutputType(), nil
}

// Eval compiles and runs an expression against real data.
func (e *Engine) Eval(env *cel.Env, expr string, in map[string]interface{}) (interface{}, error) {
	return e.run(env, expr, normaliseInput(in))
}

// run evaluates an expression against an activation already normalised.
func (e *Engine) run(env *cel.Env, expr string, activation map[string]interface{}) (interface{}, error) {
	c, err := e.compile(env, expr)
	if err != nil {
		return nil, err
	}
	out, _, err := c.prg.Eval(activation)
	if err != nil {
		return nil, err
	}
	return native(out), nil
}

// EvalProperty evaluates a whole property value, interpolation included.
//
// template.Parse splits the value into text and expression fragments:
//
//	'https://$(source.registry.host)/health'  ->  "https://" + host + "/health"
//
// A value that is a single expression keeps its type - an int stays an int - and
// one embedded in text yields a string, because concatenating with text is what
// that means.
func (e *Engine) EvalProperty(env *cel.Env, raw string, in map[string]interface{}) (interface{}, error) {
	activation := normaliseInput(in)
	return interpolate(raw, func(expr string, _ bool) (interface{}, error) {
		return e.run(env, expr, activation)
	})
}

// interpolate renders one property value, calling eval for each expression in
// it. whole reports whether the expression is the entire value.
func interpolate(raw string, eval func(expr string, whole bool) (interface{}, error)) (interface{}, error) {
	parsed, err := template.Parse(raw)
	if err != nil {
		return nil, err
	}
	// Nothing to evaluate, but `$$(` still collapses, as it does in a value that
	// also holds an expression.
	if !parsed.HasExpr() {
		return parsed.Literal(), nil
	}
	if expr, ok := parsed.SoleExpr(); ok {
		return eval(expr, true)
	}
	var b strings.Builder
	for _, f := range parsed.Fragments {
		if !f.IsExpr() {
			b.WriteString(f.Text)
			continue
		}
		v, err := eval(f.Expr, false)
		if err != nil {
			return nil, fmt.Errorf("evaluating %q: %w", f.Expr, err)
		}
		// Text embeds one value; a collection has no single spelling as text.
		switch v.(type) {
		case []interface{}:
			return nil, fmt.Errorf("%q is a list, which text cannot embed; join it into a string, "+
				"for example %s.join(\",\")", f.Expr, f.Expr)
		case map[string]interface{}:
			return nil, fmt.Errorf("%q is a map, which text cannot embed; select a value from it", f.Expr)
		}
		fmt.Fprintf(&b, "%v", v)
	}
	return b.String(), nil
}

// Typed marks a value whose numbers carry the types a schema declares, such as
// a fetched value retyped against the schema that describes it. Its widths are lined up for
// CEL, and integral floats in it are left as floats rather than read as ints.
func Typed(v interface{}) interface{} { return alreadyTyped{value: normaliseWidths(v)} }

// native converts a CEL value into ordinary Go data.
//
// ref.Val.Value() is only shallow. A field selection returns whatever was put in,
// so `source.cfg.meta` comes back as a plain map - but anything CEL *constructs*
// is built from its own types, so `{"a": x}` yields map[ref.Val]ref.Val and
// `[x, y]` yields []ref.Val. Those cannot be marshalled back into a properties
// document, which is where a substituted value has to end up.
//
// Map keys are rendered as strings because that is what a properties document
// requires; a non-string key would not survive JSON anyway.
func native(v ref.Val) interface{} {
	switch t := v.(type) {
	case traits.Lister:
		n, ok := t.Size().Value().(int64)
		if !ok {
			return v.Value()
		}
		out := make([]interface{}, 0, n)
		for i := int64(0); i < n; i++ {
			out = append(out, native(t.Get(types.Int(i))))
		}
		return out
	case traits.Mapper:
		out := map[string]interface{}{}
		for it := t.Iterator(); it.HasNext() == types.True; {
			k := it.Next()
			val := t.Get(k)
			out[fmt.Sprintf("%v", native(k))] = native(val)
		}
		return out
	default:
		return v.Value()
	}
}

// normaliseNumbers turns an integral float64 into an int64 throughout a value,
// on top of the width conversions normaliseWidths makes.
//
// It is the reading for a value with no schema to consult. Such a value has been
// through JSON, where every number decodes as float64, and CEL has no mixed
// numeric overloads: `port + 1000` against a decoded 8080 would fail with "no
// such overload". An integral float is read as the int it most likely was. A
// value with a schema is marked Typed instead, and only its widths change.
func normaliseNumbers(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, child := range t {
			out[k] = normaliseNumbers(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, child := range t {
			out[i] = normaliseNumbers(child)
		}
		return out
	case alreadyTyped:
		return t.value
	case float64:
		// Only where the int64 can hold it: 2^63 itself does not fit.
		if t == math.Trunc(t) && t >= math.MinInt64 && t < math.MaxInt64 {
			return int64(t)
		}
		return t
	default:
		return normaliseWidths(v)
	}
}

// alreadyTyped wraps a value whose numeric types are the ones its schema
// declares, so normaliseNumbers unwraps it rather than guessing over it.
type alreadyTyped struct{ value interface{} }

// normaliseWidths converts Go's numeric widths to the two CEL has overloads for,
// and nothing else.
//
// For a value typed by a schema, that is the whole job. Guessing from the value
// would read a `float` field holding 2.0 as an int, and `ratio * 2.0`, which
// type-checks as double, would fail at evaluation.
func normaliseWidths(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, child := range t {
			out[k] = normaliseWidths(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, child := range t {
			out[i] = normaliseWidths(child)
		}
		return out
	case float32:
		return float64(t)
	case int:
		return int64(t)
	case int8:
		return int64(t)
	case int16:
		return int64(t)
	case int32:
		return int64(t)
	case uint:
		return unsigned(uint64(t))
	case uint8:
		return int64(t)
	case uint16:
		return int64(t)
	case uint32:
		return int64(t)
	case uint64:
		return unsigned(t)
	default:
		return v
	}
}

// unsigned reads an unsigned value as the int it fits in, since CEL has no
// mixed int and uint overloads, and keeps it unsigned only when it does not.
func unsigned(n uint64) interface{} {
	if n <= math.MaxInt64 {
		return int64(n)
	}
	return n
}

// normaliseInput applies normaliseNumbers to an activation map, keeping the type
// CEL's Eval expects.
func normaliseInput(in map[string]interface{}) map[string]interface{} {
	out, _ := normaliseNumbers(in).(map[string]interface{})
	if out == nil {
		return in
	}
	return out
}
