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
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

func TestOptionalPath(t *testing.T) {
	schema := cuecontext.New().CompileString(`{
	host: string
	note?: string
	network?: {vpcId: string}
	labels: [string]: string
	teams: {core: string, [string]: string}
	outputs: [...{kind: string, note?: string}]
	pinned: [string, ...string]
	pair: [string, string]
	blob: _
}`)
	require.NoError(t, schema.Err())

	for _, tc := range []struct {
		path   []string
		absent bool
	}{
		{[]string{"host"}, false},
		{[]string{"note"}, true},
		{[]string{"network", "vpcId"}, true},
		{[]string{"labels", "team"}, true},
		{[]string{"outputs", "0", "kind"}, false},
		{[]string{"outputs", "3", "note"}, true},
		{[]string{"pinned", "5"}, false},
		{[]string{"pair", "1"}, false},
		{[]string{"pair", "2"}, true},
		{[]string{"outputs", "-1", "kind"}, true},
		{[]string{"pinned", "0"}, false},
		{[]string{"blob", "anything"}, false},
		{[]string{"undeclared"}, false},
		{[]string{"teams", "core"}, false},
		{[]string{"teams", "other"}, true},
		{[]string{"outputs", "kind"}, false},
	} {
		got, err := OptionalPath(schema, tc.path)
		require.NoError(t, err)
		require.Equal(t, tc.absent, got, "%v", tc.path)
	}
}
