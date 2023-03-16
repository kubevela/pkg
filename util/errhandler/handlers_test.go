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

package errhandler_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/errhandler"
)

func TestNotifyOrPanic(t *testing.T) {
	ch := make(chan error, 1)
	h1 := errhandler.NotifyOrPanic(ch)
	h2 := errhandler.NotifyOrPanic(nil)
	h1(nil)
	require.Equal(t, 0, len(ch))
	e := fmt.Errorf("err")
	h1(e)
	require.Equal(t, 1, len(ch))
	func() {
		defer func() {
			err := recover()
			require.NotNil(t, err)
		}()
		h2(e)
	}()
}
