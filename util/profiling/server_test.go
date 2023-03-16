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

package profiling_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/profiling"
)

func TestStartProfilingServer(t *testing.T) {
	fn := profiling.NewProfilingHandler()
	server := httptest.NewServer(fn)
	defer server.Close()
	_, err := http.Get(server.URL + "/mem/stat")
	require.NoError(t, err)
	_, err = http.Get(server.URL + "/gc")
	require.NoError(t, err)

	profiling.AddFlags(pflag.NewFlagSet("", pflag.ExitOnError))
	profiling.StartProfilingServer(nil)
}
