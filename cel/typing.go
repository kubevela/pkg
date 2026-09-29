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
	"cuelang.org/go/cue"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	apiservercel "k8s.io/apiserver/pkg/cel"
)

// DeclType translates a CUE schema into a CEL declared type, named name.
//
// The name must be unique across everything declared in one environment: the
// type provider keeps the first type registered under a name, so a second schema
// sharing it would have its fields reported as undefined.
func DeclType(v cue.Value, name string) *apiservercel.DeclType {
	switch v.IncompleteKind() {
	case cue.StringKind:
		return apiservercel.StringType
	case cue.IntKind:
		return apiservercel.IntType
	case cue.FloatKind, cue.NumberKind:
		return apiservercel.DoubleType
	case cue.BoolKind:
		return apiservercel.BoolType
	case cue.ListKind:
		if elem := v.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
			return apiservercel.NewListType(DeclType(elem, name+".item"), -1)
		}
		return apiservercel.NewListType(apiservercel.AnyType, -1)
	case cue.StructKind:
		// An open map - `[string]: string` - is a CEL map. A closed struct with
		// named fields is a CEL object. The distinction matters: a map read may
		// be absent (has() applies), an object field is declared.
		if elem := v.LookupPath(cue.MakePath(cue.AnyString)); elem.Exists() {
			return apiservercel.NewMapType(apiservercel.StringType, DeclType(elem, name+".value"), -1)
		}
		fields := map[string]*apiservercel.DeclField{}
		iter, err := v.Fields(cue.Optional(true))
		if err != nil {
			return apiservercel.AnyType
		}
		for iter.Next() {
			f := iter.Selector().Unquoted()
			fields[f] = apiservercel.NewDeclField(
				f, DeclType(iter.Value(), name+"."+f), !iter.IsOptional(), nil, nil)
		}
		return apiservercel.NewObjectType(name, fields)
	default:
		return apiservercel.AnyType
	}
}

// ElementsCompatible reports whether a collection-valued expression can feed a
// collection-valued parameter.
//
// A CUE kind says only "list", so a kind check cannot tell list(string) from a
// `[...int]` parameter. CEL carries the element type, so the elements are
// compared here.
//
// It judges collections only, and only where both sides are concrete. A struct,
// an untyped region and a `dyn` all fail open, because that is where a legitimate
// widening is most likely and a false rejection is unfixable by the expression's author.
// The outer kind check has already run; this only narrows within a matching kind.
//
// Returns the target and expression types when they cannot agree, for the message.
func ElementsCompatible(src *cel.Type, dst cue.Value) (bool, string, string) {
	if src == nil || !dst.Exists() {
		return true, "", ""
	}
	want := DeclType(dst, "target").CelType()
	if elementsAgree(src, want) {
		return true, "", ""
	}
	return false, want.String(), src.String()
}

func elementsAgree(src, dst *cel.Type) bool {
	if src == nil || dst == nil || failsOpen(src) || failsOpen(dst) {
		return true
	}
	sp, dp := src.Parameters(), dst.Parameters()
	switch {
	case src.Kind() == types.ListKind && dst.Kind() == types.ListKind:
		if len(sp) == 1 && len(dp) == 1 {
			return elementsAgree(sp[0], dp[0]) && scalarsAgree(sp[0], dp[0])
		}
	case src.Kind() == types.MapKind && dst.Kind() == types.MapKind:
		if len(sp) == 2 && len(dp) == 2 {
			return scalarsAgree(sp[0], dp[0]) && elementsAgree(sp[1], dp[1]) && scalarsAgree(sp[1], dp[1])
		}
	}
	return true
}

// scalarsAgree compares two element types once collections have been unwrapped.
func scalarsAgree(src, dst *cel.Type) bool {
	if failsOpen(src) || failsOpen(dst) {
		return true
	}
	if src.Kind() == dst.Kind() {
		return true
	}
	// Any integer is a valid double, and a CUE int covers both signs; a double
	// may carry a fraction no integer can.
	integer := map[types.Kind]bool{types.IntKind: true, types.UintKind: true}
	return integer[src.Kind()] && (integer[dst.Kind()] || dst.Kind() == types.DoubleKind)
}

// failsOpen reports the types this check refuses to judge.
func failsOpen(t *cel.Type) bool {
	//nolint:exhaustive // an allowlist of the kinds too loose to judge, not a mapping of every kind
	switch t.Kind() {
	case types.DynKind, types.AnyKind, types.StructKind, types.OpaqueKind, types.TypeParamKind:
		return true
	}
	return false
}
