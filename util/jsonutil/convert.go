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
	"encoding/json"
	"fmt"
)

// AsType call json marshal and unmarshal to convert src to given target type
func AsType[T any](src interface{}) (*T, error) {
	bs, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %T: %w", src, err)
	}
	dest := new(T)
	if err = json.Unmarshal(bs, dest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to %T: %w", dest, err)
	}
	return dest, nil
}

// CopyInto marshal the source object and unmarshal to destination object
func CopyInto(src interface{}, dest interface{}) error {
	bs, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("failed to marshal %T: %w", src, err)
	}
	return json.Unmarshal(bs, dest)
}
