/*
Copyright 2023 The KubeVela Authors.

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

package util

import (
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
)

const orderKey = "step"

// Iterate over all fields of the cue.Value with fn, if fn returns true,
// iteration stops
func Iterate(value cue.Value, fn func(v cue.Value) (stop bool)) (stop bool) {
	// skip definition
	if strings.Contains(value.Path().String(), "#") {
		return false
	}
	return iterate(value, fn)
}

// iterate walks the value testing the selector it descends through, rather
// than rebuilding and stringifying each node's whole path from the root to
// find a "#" in it.
func iterate(value cue.Value, fn func(cue.Value) bool) bool {
	for _, f := range fields(value) {
		if isDefinition(f.selector) {
			continue
		}
		if iterate(f.value, fn) {
			return true
		}
	}
	return fn(value)
}

// isDefinition reports what the walk used to test as a "#" anywhere in the
// node's path. Checking the one selector being descended into is equivalent,
// since the walk never reaches a node without descending through its parents.
func isDefinition(sel cue.Selector) bool {
	if sel.IsDefinition() {
		return true
	}
	// a quoted label may still spell a "#" without being a definition
	return sel.LabelType() == cue.StringLabel && strings.Contains(sel.Unquoted(), "#")
}

type field struct {
	selector cue.Selector
	value    cue.Value
}

// fields returns the field values of the given value paired with the selector
// that reaches them, in the order FieldValues would return them.
func fields(value cue.Value) []field {
	var out []field
	switch value.Kind() {
	case cue.ListKind:
		it, err := value.List()
		if err != nil {
			return nil
		}
		for i := 0; it.Next(); i++ {
			out = append(out, field{cue.Index(i), it.Value()})
		}
	default:
		// Most of a rendered manifest is the strings and numbers it is made
		// of, and asking each of them for its fields means sorting a struct
		// that is not there: Fields finalises the node and orders its labels
		// before reporting that a scalar has none.
		//
		// Bottom is not a scalar for this purpose. A node whose contents
		// conflict reports it whatever it was going to hold, so it is still
		// asked and still reports the conflict.
		if kind := value.IncompleteKind(); kind != cue.BottomKind && kind&cue.StructKind == 0 {
			return nil
		}
		it, err := value.Fields(cue.Optional(true), cue.Hidden(true))
		if err != nil {
			return nil
		}
		for it.Next() {
			out = append(out, field{it.Selector(), it.Value()})
		}
	}
	return sortByOrder(out)
}

// ByStepOrder puts values in the order their "step" attributes ask for,
// leaving them as they are where none of them has one.
//
// Iterate does this to a value's own fields on its way down. A caller that
// has picked out some of those fields itself, and iterates each of them in
// turn, never passes through that and has to ask for the order here.
func ByStepOrder(values []cue.Value) []cue.Value {
	in := make([]field, len(values))
	for i, v := range values {
		in[i] = field{value: v}
	}
	out := make([]cue.Value, len(values))
	for i, f := range sortByOrder(in) {
		out[i] = f.value
	}
	return out
}

// sortByOrder orders fields by their "step" attribute. Templates that use no
// step attribute - which is almost all of them - skip the sort entirely rather
// than pay an attribute lookup per comparison.
func sortByOrder(in []field) []field {
	ordered := false
	for i := range in {
		attr := in[i].value.Attribute(orderKey)
		if attr.Err() == nil {
			ordered = true
			break
		}
	}
	if !ordered {
		return in
	}
	sort.SliceStable(in, func(i, j int) bool {
		xAttr, yAttr := in[i].value.Attribute(orderKey), in[j].value.Attribute(orderKey)
		x, e1 := strconv.ParseInt(xAttr.Contents(), 10, 32)
		y, e2 := strconv.ParseInt(yAttr.Contents(), 10, 32)
		switch {
		case e1 != nil && e2 != nil:
			return false
		case e1 != nil:
			return false
		case e2 != nil:
			return true
		default:
			return x < y
		}
	})
	return in
}

// FieldValues the field values of the given value
// If the given value is a list, all its items will be returned
// If the given value is a map, all its key-value entries will be returned
// The returned values will be sorted in the order of their "step" attribute
func FieldValues(value cue.Value) []cue.Value {
	fs := fields(value)
	values := make([]cue.Value, len(fs))
	for i, f := range fs {
		values[i] = f.value
	}
	return values
}
