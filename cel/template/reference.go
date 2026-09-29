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

package template

import (
	"fmt"
	"strings"
)

// Reference is one path an expression reads.
type Reference struct {
	// Root is the top-level name the read starts from.
	Root string
	// Path is the rest, one segment per field, index or qualifier.
	Path []string
	// Defaulted records that the read is guarded against absence, by has(),
	// `"k" in m`, or a ternary arm, so it survives the value being missing.
	Defaulted bool
}

// String renders a reference the way an error message names it.
//
// Segments are rendered so the result is a valid expression, because the errors
// that use it tell the author what to write, such as a has() guard around the
// read. Joined with dots, an index would come out as `outputs.0.name`, which
// does not parse.
//
// A segment is rendered from its text alone, so a key that is all digits, such as
// cfg["0"], renders as an index, cfg[0]. Lookups decide by the value's kind and
// are unaffected; only the rendered name is ambiguous.
func (r Reference) String() string {
	var b strings.Builder
	b.WriteString(r.Root)
	for _, segment := range r.Path {
		if q, ok := SegmentQualifier(segment); ok {
			b.WriteString(callExpr(q))
			continue
		}
		switch {
		case isIndexSegment(segment):
			fmt.Fprintf(&b, "[%s]", segment)
		case isIdent(segment):
			b.WriteString("." + segment)
		default:
			// A hyphenated name or a label key with a dot in it. Bracket syntax
			// is the only form that reads these at all.
			fmt.Fprintf(&b, "[%q]", segment)
		}
	}
	return b.String()
}

// qualifierSegment marks a qualifier inside a read path, so the path stays one
// list of segments and every comparison over paths keeps working. A NUL cannot
// appear in a field an author could write.
const qualifierSegment = "\x00"

// QualifierSegment encodes a qualifier as a path segment.
func QualifierSegment(q string) string { return qualifierSegment + q }

// SegmentQualifier reports the qualifier a path segment encodes, if it is one.
func SegmentQualifier(segment string) (string, bool) {
	if strings.HasPrefix(segment, qualifierSegment) {
		return strings.TrimPrefix(segment, qualifierSegment), true
	}
	return "", false
}

// Call names one qualifier call, fn("arg"), as it is recorded, delivered and
// looked up.
func Call(fn, arg string) string { return fn + ":" + arg }

// SplitCall is the inverse of Call.
func SplitCall(call string) (string, string) {
	fn, arg, _ := strings.Cut(call, ":")
	return fn, arg
}

// callExpr renders a qualifier call the way an author writes it.
func callExpr(call string) string {
	fn, arg := SplitCall(call)
	return fmt.Sprintf(".%s(%q)", fn, arg)
}

// isIndexSegment reports a segment that came from a list index. Indices are
// recorded as decimal text, and nothing else in a path is all digits: a field
// cannot start with one.
func isIndexSegment(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// keywords are identifiers CEL will not select with a dot.
var keywords = map[string]bool{"in": true, "null": true, "true": true, "false": true}

// isIdent reports a segment CEL can select with a dot.
func isIdent(s string) bool {
	if s == "" || keywords[s] {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
