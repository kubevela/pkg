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

package stringtools

import (
	"regexp"
	"strings"

	"github.com/kubevela/pkg/util/slices"
)

var indentPattern = regexp.MustCompile(`^(\s*)\S`)

// TrimLeadingIndent remove leading indent from each line of given string
// The indent size is extracted from the leading space of the first line.
func TrimLeadingIndent(s string) string {
	s = strings.Trim(s, "\n")
	lines := strings.Split(s, "\n")
	firstLineIdx := slices.Index(lines, func(s string) bool { return len(strings.TrimSpace(s)) > 0 })
	if firstLineIdx < 0 {
		return ""
	}
	lines = lines[firstLineIdx:]
	match := indentPattern.FindStringSubmatch(lines[0])
	if len(match) < 2 {
		return s
	}
	indent := match[1]
	return strings.TrimSpace(strings.Join(slices.Map(lines, func(line string) string {
		return strings.TrimPrefix(line, indent)
	}), "\n"))
}
