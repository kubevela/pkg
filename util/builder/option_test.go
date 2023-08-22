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

package builder_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/builder"
)

type B struct {
	key string
	val string
}

func (in *B) New() *B {
	return &B{key: "key"}
}

type Suffix string

func (in Suffix) ApplyTo(b *B) {
	b.key += string(in)
}

func TestOption(t *testing.T) {
	opt := builder.OptionFn[B](func(b *B) { b.val = "val" })
	b := builder.NewOptions[B](opt, Suffix("-x"))
	require.Equal(t, "key-x", b.key)
	require.Equal(t, "val", b.val)
}
