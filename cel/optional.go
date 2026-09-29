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
	"strconv"

	"cuelang.org/go/cue"
)

// OptionalPath reports whether any segment of a path may be absent at render.
//
// Any segment, not just the last: if `network?: {vpcId: string}` then
// network.vpcId is absent whenever network is, however required vpcId looks
// inside it.
//
// A key read out of an open map counts too: `labels: [string]: string` declares
// the map, never a key, so `labels["team"]` may find nothing.
func OptionalPath(v cue.Value, path []string) (bool, error) {
	cur := v
	for _, segment := range path {
		if cur.IncompleteKind() == cue.TopKind {
			// Inside a `_` field. Whether this key exists is unknowable here, so it
			// is not reported: demanding a default for every read below an untyped
			// region would be noise. An absent key there surfaces at evaluation.
			return false, nil
		}
		if cur.IncompleteKind() == cue.ListKind {
			// A list index. Past the end of a fixed-length list, `[string, string]`,
			// the element cannot exist. In an open list, `[...{kind: string}]`,
			// whether it does is for the expression to check with size(), which no
			// has() guard can express, so the index is taken as present and the
			// element's own fields are judged.
			index, err := strconv.Atoi(segment)
			if err != nil {
				//nolint:nilerr // a non-numeric segment is not an index, not a failure
				return false, nil
			}
			if index < 0 {
				// CEL does not index from the end, so no element is there.
				return true, nil
			}
			if pinned := cur.LookupPath(cue.MakePath(cue.Index(index))); pinned.Exists() {
				cur = pinned
				continue
			}
			if elem := cur.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
				cur = elem
				continue
			}
			return true, nil
		}
		if pattern := cur.LookupPath(cue.MakePath(cue.AnyString)); pattern.Exists() {
			if !cur.LookupPath(cue.MakePath(cue.Str(segment))).Exists() {
				return true, nil
			}
		}
		optional, next, found, err := fieldByName(cur, segment)
		if err != nil {
			return false, err
		}
		if !found {
			// An undeclared field: the type check reports that, better.
			return false, nil
		}
		if optional {
			return true, nil
		}
		cur = next
	}
	return false, nil
}

func fieldByName(v cue.Value, name string) (optional bool, value cue.Value, found bool, err error) {
	iter, ierr := v.Fields(cue.Optional(true))
	if ierr != nil {
		return false, cue.Value{}, false, ierr
	}
	for iter.Next() {
		if iter.Selector().Unquoted() == name {
			return iter.IsOptional(), iter.Value(), true, nil
		}
	}
	return false, cue.Value{}, false, nil
}
