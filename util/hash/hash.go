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

package hash

import (
	"strconv"

	"github.com/mitchellh/hashstructure/v2"
)

// ComputeHash computes the hash value of a input
func ComputeHash(input interface{}) (string, error) {
	// compute a hash value of any resource spec
	val, err := hashstructure.Hash(input, hashstructure.FormatV2, nil)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(val, 16), nil
}
