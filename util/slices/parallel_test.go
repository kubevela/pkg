/*
Copyright 2022 The KubeVela Authors.

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

package slices_test

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/kubevela/pkg/util/slices"
)

func TestParMap(t *testing.T) {
	type input struct {
		a int
		b int
		c float64
		d *int
	}
	var inputs []input
	size := 100
	parallelism := 20
	for i := 0; i < size; i++ {
		if i%2 == 0 {
			inputs = append(inputs, input{i, i + 1, math.Sqrt(float64(i)), nil})
		} else {
			inputs = append(inputs, input{i, i + 1, math.Sqrt(float64(i)), ptr.To(i)})
		}
	}
	outputs := slices.ParMap(inputs, func(i input) *float64 {
		time.Sleep(time.Duration(rand.Intn(200)+25) * time.Millisecond)
		if i.d == nil {
			return nil
		}
		return ptr.To(float64(i.a*i.b) + i.c + float64(*i.d))
	}, slices.Parallelism(parallelism))
	r := require.New(t)
	r.Equal(size, len(outputs))
	for i, j := range outputs {
		if i%2 == 0 {
			r.Nil(j)
		} else {
			r.NotNil(j)
			r.Equal(float64(i*(i+1))+math.Sqrt(float64(i))+float64(i), *j)
		}
	}
}

func TestParFor(t *testing.T) {
	type input struct{ key int }
	var inputs []input
	size := 100
	for i := 0; i < size; i++ {
		inputs = append(inputs, input{i})
	}
	slices.ParFor(inputs, func(obj input) {
		time.Sleep(time.Duration(rand.Intn(50)+size-obj.key) * time.Millisecond)
	}, slices.Parallelism(20))
}
