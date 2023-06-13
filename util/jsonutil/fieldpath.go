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

package jsonutil

import (
	"fmt"
	"strconv"

	"github.com/kubevela/pkg/util/slices"
)

// Field if Index is not null, it points to an array index; otherwise a
// struct field
type Field struct {
	Label string
	Index *int64
}

// ParseFields extract fields from one path
func ParseFields(path string) (fields []Field, err error) {
	chars := []rune(path)
	i, s := 0, 0
	for i < len(chars) {
		quoted := chars[i] == '"'
		if quoted {
			i += 1
		}
		s = i
		e := -1
		for i < len(chars) {
			if quoted && chars[i] == '"' {
				if i+1 < len(chars) && chars[i+1] != '.' {
					return nil, fmt.Errorf("unexpected char '%c' at position %d", chars[i+1], i+1)
				}
				e = i
				i += 2
				break
			}
			if !quoted && chars[i] == '.' {
				if i+1 >= len(chars) {
					return nil, fmt.Errorf("unexpected end period '.' at position %d", i+1)
				}
				e = i
				i += 1
				break
			}
			if chars[i] == '\\' {
				i += 1
				if i+1 >= len(chars) {
					return nil, fmt.Errorf("unexpected slash '\\' at position %d", i+1)
				}
			}
			i += 1
		}
		if e < 0 {
			e = len(chars)
		}
		if s == e {
			return nil, fmt.Errorf("empty field found at position %d", i)
		}
		field := Field{Label: path[s:e]}
		if !quoted && slices.All(chars[s:e], func(r rune) bool { return r >= '0' && r <= '9' }) {
			idx, _ := strconv.ParseInt(path[s:e], 10, 64)
			field.Index = &idx
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// LookupPath lookup path for obj
func LookupPath(obj any, path string) (any, error) {
	defer func() { recover() }()
	fields, err := ParseFields(path)
	if err != nil {
		return nil, err
	}
	cur := obj
	for _, field := range fields {
		if cur == nil {
			break
		}
		switch o := cur.(type) {
		case map[string]any:
			cur = o[field.Label]
		case []any:
			if idx := field.Index; idx != nil && *idx >= 0 && int(*idx) < len(o) {
				cur = o[*idx]
			} else {
				cur = nil
			}
		default:
			cur = nil
		}
	}
	return cur, nil
}
