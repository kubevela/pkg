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
	"strings"

	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"

	"cuelang.org/go/cue"
	"github.com/google/cel-go/cel"
)

// A Target is the schema a tree's values land in: the parameter block of
// whatever the properties feed.
type Target interface {
	// At is the schema at property, whether the property is required, and
	// whether the target declares it at all; an open struct may not.
	At(property string) (schema cue.Value, required, declared bool)
}

// TargetFunc adapts a function to a Target.
type TargetFunc func(property string) (cue.Value, bool, bool)

// At calls f.
func (f TargetFunc) At(property string) (cue.Value, bool, bool) { return f(property) }

// TypeOptions adjusts CheckTypes.
type TypeOptions struct {
	// Optional reports a read that may be missing at render. Such a read,
	// unguarded, is refused where it feeds a required property.
	Optional func(Read) bool
	// DeclaredKind is the kind the host's schema declares for a read, when it
	// knows one. CEL types CUE's number as double, so an expression that is
	// exactly one read is judged by this kind instead; a computed result keeps
	// CEL's type.
	DeclaredKind func(Read) (cue.Kind, bool)
}

// A TypeMismatch is an expression whose result does not fit its target.
type TypeMismatch struct {
	// Got and Want are kind names, or element types when Elements is set.
	Got, Want string
	// Elements reports that the kinds agree, both lists or both maps, but
	// their element types do not.
	Elements bool
}

func (m *TypeMismatch) Error() string {
	return fmt.Sprintf("type mismatch: the expression is %s but the target expects %s", m.Got, m.Want)
}

// A MayBeAbsent is an unguarded read that may be missing at render, feeding a
// required property.
type MayBeAbsent struct{ Read Read }

func (m *MayBeAbsent) Error() string {
	return fmt.Sprintf("%s may be absent and feeds a required property", m.Read)
}

// CheckTypes compiles each property's expressions against env, whose roots
// carry the types of what they read, and asserts the result fits the target
// at that property. It reports at most one fault per property, in this order:
// an expression that does not compile there; a collection embedded in text; a
// result whose kind, or whose elements, do not fit; an unguarded read that may
// be missing feeding a required property.
//
// A dyn result, a read below an untyped region or of a value with no schema,
// has no type to compare and fits anything; a property the target does not
// declare is not judged. With no target, only what env refuses is reported.
func (p *Plan) CheckTypes(env *cel.Env, target Target, opts TypeOptions) []CheckError {
	var out []CheckError
	for _, prop := range p.properties() {
		xs := p.expressionsAt(prop)
		kind, t, err := p.resultOf(env, xs, opts)
		if err != nil {
			out = append(out, CheckError{Property: prop, Expr: xs[0].Expr, Err: err})
			continue
		}
		if target == nil {
			continue
		}
		schema, required, declared := target.At(prop)
		if !declared {
			continue
		}
		dst := schema.IncompleteKind()
		if !kindsCompatible(kind, dst) {
			out = append(out, CheckError{Property: prop, Expr: xs[0].Expr,
				Err: &TypeMismatch{Got: kindName(kind), Want: kindName(dst)}})
			continue
		}
		if ok, want, got := ElementsCompatible(t, schema); !ok {
			out = append(out, CheckError{Property: prop, Expr: xs[0].Expr,
				Err: &TypeMismatch{Got: got, Want: want, Elements: true}})
			continue
		}
		if !required || opts.Optional == nil {
			continue
		}
		if r, ok := undefended(xs, opts.Optional); ok {
			out = append(out, CheckError{Property: prop, Read: r, Err: &MayBeAbsent{Read: r}})
		}
	}
	return out
}

// resultOf is the kind a property's value takes, and its CEL type when the
// value is a single expression. Expressions embedded in text make a string,
// and each must have a string form.
func (p *Plan) resultOf(env *cel.Env, xs []Expression, opts TypeOptions) (cue.Kind, *cel.Type, error) {
	e := p.engine
	if len(xs) == 1 && xs[0].Whole {
		x := xs[0]
		t, err := e.OutputType(env, x.Expr)
		if err != nil {
			return cue.BottomKind, nil, err
		}
		if opts.DeclaredKind != nil && len(x.Reads) == 1 && e.bareRead(x.Expr) {
			if k, ok := opts.DeclaredKind(x.Reads[0]); ok {
				return k, t, nil
			}
		}
		return celKind(t), t, nil
	}
	for _, x := range xs {
		t, err := e.OutputType(env, x.Expr)
		if err != nil {
			return cue.BottomKind, nil, err
		}
		//nolint:exhaustive // only the kinds with no string form are refused
		switch celKind(t) {
		case cue.StructKind, cue.ListKind:
			return cue.BottomKind, nil, fmt.Errorf(
				"expression $(%s) is %s and cannot be combined with text; read a field out of it", x.Expr, t)
		}
	}
	return cue.StringKind, nil, nil
}

func undefended(xs []Expression, optional func(Read) bool) (Read, bool) {
	for _, x := range xs {
		for _, r := range x.Reads {
			if !r.Defaulted && optional(r) {
				return r, true
			}
		}
	}
	return Read{}, false
}

// properties returns each property holding an expression, in plan order.
func (p *Plan) properties() []string {
	var out []string
	for _, x := range p.exprs {
		if len(out) == 0 || out[len(out)-1] != x.Property {
			out = append(out, x.Property)
		}
	}
	return out
}

func (p *Plan) expressionsAt(property string) []Expression {
	var out []Expression
	for _, x := range p.exprs {
		if x.Property == property {
			out = append(out, x)
		}
	}
	return out
}

// celKind maps a CEL type onto the CUE kind a target is compared against. dyn
// and any become TopKind, which fits every kind.
func celKind(t *cel.Type) cue.Kind {
	switch t.String() {
	case "string":
		return cue.StringKind
	case "int", "uint":
		return cue.IntKind
	case "double":
		return cue.FloatKind
	case "bool":
		return cue.BoolKind
	case "null_type":
		return cue.NullKind
	case "dyn", "any":
		return cue.TopKind
	}
	if strings.HasPrefix(t.String(), "list(") {
		return cue.ListKind
	}
	return cue.StructKind
}

// KindFits reports whether a value of kind src fits a target of kind dst. Kinds
// that share a member fit, so a number fits an int, since it may hold one; an
// integer fits a number; a float does not fit an int, since it may carry a
// fraction. Bottom, meaning unknown, fits anything.
func KindFits(src, dst cue.Kind) bool { return kindsCompatible(src, dst) }

func kindsCompatible(src, dst cue.Kind) bool {
	if src == cue.BottomKind || dst == cue.BottomKind {
		return true
	}
	if src&dst != 0 {
		return true
	}
	return src == cue.IntKind && dst&(cue.NumberKind|cue.FloatKind) != 0
}

func kindName(k cue.Kind) string {
	switch k {
	case cue.StringKind:
		return "string"
	case cue.IntKind:
		return "int"
	case cue.NumberKind, cue.FloatKind:
		return "number"
	case cue.BoolKind:
		return "bool"
	case cue.StructKind:
		return "object"
	case cue.ListKind:
		return "list"
	case cue.NullKind:
		return "null"
	}
	return k.String()
}

// bareRead reports whether an expression is nothing but one read: fields,
// literal indices and qualifiers from a root, with no computation on the value.
func (e *Engine) bareRead(expr string) bool {
	c, err := e.compile(e.dyn, expr)
	if err != nil {
		return false
	}
	cur := c.ast.NativeRep().Expr()
	for {
		switch cur.Kind() {
		case celast.SelectKind:
			if cur.AsSelect().IsTestOnly() {
				return false
			}
			cur = cur.AsSelect().Operand()
		case celast.CallKind:
			call := cur.AsCall()
			if _, ok := e.qualifier(call); ok {
				cur = call.Target()
				continue
			}
			if call.FunctionName() != operators.Index || len(call.Args()) != 2 || call.Args()[1].Kind() != celast.LiteralKind {
				return false
			}
			cur = call.Args()[0]
		case celast.IdentKind:
			return e.isRoot[cur.AsIdent()]
		default:
			return false
		}
	}
}
