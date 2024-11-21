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

package apiserver_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/apiserver"
)

func TestGetMetadataNameInFieldSelectorFromInternalVersionListOptions(t *testing.T) {
	options := &metainternalversion.ListOptions{}
	require.True(t, nil == apiserver.GetMetadataNameInFieldSelectorFromInternalVersionListOptions(options))
	options.FieldSelector = fields.SelectorFromSet(fields.Set{"metadata.name": "val"})
	require.Equal(t, ptr.To("val"), apiserver.GetMetadataNameInFieldSelectorFromInternalVersionListOptions(options))
}

func TestBuildQueryParamsFromLabelSelector(t *testing.T) {
	sel := labels.NewSelector()
	r1, err := labels.NewRequirement("a", selection.Equals, []string{"x"})
	require.NoError(t, err)
	r2, err := labels.NewRequirement("b", selection.In, []string{"y", "z"})
	require.NoError(t, err)
	r3, err := labels.NewRequirement("c", selection.NotEquals, []string{"t"})
	require.NoError(t, err)
	r4, err := labels.NewRequirement("d", selection.Equals, []string{"t"})
	require.NoError(t, err)
	sel = sel.Add(*r1, *r2, *r3, *r4)
	require.Equal(t, "&a=x&b=y,z", apiserver.BuildQueryParamsFromLabelSelector(sel, "a", "b"))
}

func TestNewMatchingLabelSelectorFromInternalVersionListOptions(t *testing.T) {
	opts := &metainternalversion.ListOptions{}
	opts.LabelSelector = labels.SelectorFromSet(map[string]string{"key": "val"})
	_opts := apiserver.NewMatchingLabelSelectorFromInternalVersionListOptions(opts)
	require.True(t, _opts.Matches(labels.Set{"key": "val"}))
	require.False(t, _opts.Matches(labels.Set{"key": "value"}))
}

func TestListOptions(t *testing.T) {
	opts := apiserver.NewListOptions(client.InNamespace("test"))
	_opts := apiserver.NewListOptions(opts)
	require.Equal(t, "test", _opts.Namespace)
}

func TestGetStringFromRawExtension(t *testing.T) {
	d := &runtime.RawExtension{}
	require.NoError(t, json.Unmarshal([]byte(`{"key":{"k":"value"}}`), d))
	require.Equal(t, "value", apiserver.GetStringFromRawExtension(d, "key", "k"))
	require.Equal(t, "", apiserver.GetStringFromRawExtension(d, "key", "out"))
}
