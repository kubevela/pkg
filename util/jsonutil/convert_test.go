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

package jsonutil_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/jsonutil"
)

type TestA struct {
	A int
}

func (in *TestA) MarshalJSON() ([]byte, error) {
	if in.A > 0 {
		return []byte(fmt.Sprintf(`{"val":%d}`, in.A)), nil
	}
	return nil, fmt.Errorf("val must be greater than 0")
}

type TestB struct {
	B float64 `json:"val"`
}

func (in *TestB) UnmarshalJSON(bs []byte) error {
	obj := map[string]interface{}{}
	_ = json.Unmarshal(bs, &obj)
	val, ok := obj["val"].(float64)
	if !ok || val < 2 {
		return fmt.Errorf("invalid val")
	}
	in.B = val
	return nil
}

func TestAsType(t *testing.T) {
	dest, err := jsonutil.AsType[TestB](&TestA{A: 3})
	require.NoError(t, err)
	require.Equal(t, 3, int(dest.B))

	_, err = jsonutil.AsType[TestB](&TestA{A: 0})
	require.Error(t, err)
	_, err = jsonutil.AsType[TestB](&TestA{A: 1})
	require.Error(t, err)
}
