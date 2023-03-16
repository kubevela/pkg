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

package resourcetopology

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/singleton"
)

func newDeployment(name string, namespace string, label string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"label": label},
		},
	}
}

func TestGetSubResources(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	ctx := context.Background()
	defaultIdentifier := k8s.ResourceIdentifier{
		Group:     "apps",
		Resource:  "deployment",
		Name:      "test-deploy",
		Namespace: "default",
	}
	cli := fake.NewClientBuilder().WithObjects(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "default",
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-stateful",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "test-deploy",
						Kind: "Deployment",
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "test-stateful",
						Kind: "StatefulSet",
					},
				},
			},
		},
	).WithRESTMapper(mapper).Build()
	singleton.KubeClient.Set(cli)
	singleton.RESTMapper.Set(mapper)
	cuex.EnableExternalPackageForDefaultCompiler = false

	// test new
	_ = New("")

	// Test Cases
	testCases := []struct {
		resource    k8s.ResourceIdentifier
		rt          *engine
		expected    []SubResource
		expectedErr string
	}{
		{
			resource: k8s.ResourceIdentifier{
				Group:     "apps",
				Resource:  "deployment",
				Name:      "not-found",
				Namespace: "default",
			},
			expectedErr: "not found",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
import "invalid"
`,
			},
			expectedErr: "undefined",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
}]
`,
			},
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	subResources: [{
		group: "invalid"
		resource: "statefulSet",
		selectors: ownerReference: true
	}]
}]
`,
			},
			expectedErr: "no matches for invalid",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	subResources: [{
		group: "apps",
		resource: "statefulSet",
		selectors: {
			namespace: context.data.metadata.namespace,
			ownerReference: true,
		},
	}],
}, {
	group: "apps",
	resource: "statefulSet",
	subResources: [{
		group: "invalid",
		resource: "pod",
		selectors: {
			namespace: context.data.metadata.namespace,
			ownerReference: true,
		},
	}],
}]
`,
			},
			expectedErr: "no matches for invalid",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment"
	subResources: true
}]
`,
			},
			expectedErr: "subResources should be a list",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	subResources: [{
		group: "apps",
		resource: "statefulSet",
		selectors: {
			namespace: context.data.metadata.namespace,
			ownerReference: true,
		},
	}],
}, {
	group: "apps",
	resource: "statefulSet",
	subResources: [{
		group: "",
		resource: "pod",
		selectors: {
			namespace: context.data.metadata.namespace,
			ownerReference: true,
		},
	}],
}]
`,
			},
			expected: []SubResource{
				{
					ResourceIdentifier: k8s.ResourceIdentifier{
						Group:     "apps",
						Resource:  "statefulSet",
						Name:      "test-stateful",
						Namespace: "default",
					},
					Children: []SubResource{
						{
							ResourceIdentifier: k8s.ResourceIdentifier{
								Group:     "",
								Resource:  "pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
				},
			},
		},
	}

	// Run Tests
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			r := require.New(t)
			subs, err := tc.rt.GetSubResources(ctx, tc.resource)
			if tc.expectedErr != "" {
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)
			r.Equal(tc.expected, subs)
		})
	}
}

func TestGetResourcePeers(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithObjects(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cm",
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								VolumeMounts: []corev1.VolumeMount{
									{
										Name: "mount1",
									},
									{
										Name: "mount2",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "volume-cm1",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "cm1",
										},
									},
								},
							},
							{
								Name: "volume-cm2",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "cm2",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-slice",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "Service",
						Name: "test-svc",
					},
				},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					TargetRef: &corev1.ObjectReference{
						Kind:      "Pod",
						Namespace: "default",
						Name:      "test-pod",
					},
				},
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-stateful",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "test-deploy",
						Kind: "Deployment",
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "test-stateful",
						Kind: "StatefulSet",
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod-not-match",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "test-stateful-not-match",
						Kind: "StatefulSet",
					},
				},
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cm1",
				Namespace: "cm",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cm2",
				Namespace: "default",
				Annotations: map[string]string{
					"anno": "value",
				},
				Labels: map[string]string{
					"label": "value2",
				},
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cm2-not-match",
				Namespace: "default",
				Annotations: map[string]string{
					"anno": "no-match",
				},
				Labels: map[string]string{
					"label": "value2",
				},
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cm3",
				Namespace: "default",
				Labels: map[string]string{
					"label": "value",
				},
			},
		},
	).WithRESTMapper(mapper).Build()
	singleton.KubeClient.Set(cli)
	singleton.RESTMapper.Set(mapper)
	cuex.EnableExternalPackageForDefaultCompiler = false

	defaultIdentifier := k8s.ResourceIdentifier{
		Group:     "apps",
		Resource:  "deployment",
		Name:      "test-deploy",
		Namespace: "default",
	}

	// Test Cases
	testCases := []struct {
		resource    k8s.ResourceIdentifier
		rt          *engine
		expected    []k8s.ResourceIdentifier
		expectedErr string
	}{
		{
			resource: k8s.ResourceIdentifier{
				Group:     "apps",
				Resource:  "deployment",
				Name:      "not-found",
				Namespace: "default",
			},
			expectedErr: "not found",
		},
		{
			resource: k8s.ResourceIdentifier{
				Group:     "invalid",
				Resource:  "deployment",
				Name:      "test-deploy",
				Namespace: "default",
			},
			expectedErr: "no matches for invalid",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [{
		group: "core",
		resource: "configMap",
		selectors: {
			invalid: true
		},
	}],
}]
`,
			},
			expectedErr: "unsupported selector",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment"
}]
`,
			},
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment"
	peerResources: true
}]
`,
			},
			expectedErr: "peerResources should be a list",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [1]
`,
			},
			expectedErr: "cannot use value 1",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: true
`,
			},
			expectedErr: "rules should be a list",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [{
		group: "core",
		resource: "configMap",
		selectors: {
			builtin: "invalid"
		},
	}],
}]
`,
			},
			expectedErr: "unsupported built-in rule",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [{
		group: "core",
		resource: "configMap",
		selectors: {
			namespace: 1
		},
	}],
}]
`,
			},
			expectedErr: "cannot use value 1 (type int) as string",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [{
		group: "core",
		resource: "configMap",
		selectors: {
			builtin: 1
		},
	}],
}]
`,
			},
			expectedErr: "cannot use value 1 (type int) as string",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [{
		group: "core",
		resource: "configMap",
	}],
}]
`,
			},
			expectedErr: "selectors are required",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [{
		group: "core",
		resource: "configMap",
		selectors: name: true
	}],
}]
`,
			},
			expectedErr: "cannot use value true (type bool) as list",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [{
		group: "core",
		resource: "configMap",
		selectors: true
	}],
}]
`,
			},
			expectedErr: "cannot use value true (type bool) as struct",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [true],
}]
`,
			},
			expectedErr: "cannot use value true (type bool) as struct",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: invalid: _|_
`,
			},
			expectedErr: "explicit error",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
invalid: "no-rule"
`,
			},
			expectedErr: "no rules found",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	subResources: [{
		group: "invalid",
		resource: "statefulSet",
		selectors: {
			ownerReference: true,
		},
	}],
	peerResources: [{
		group: "",
		resource: "service",
		selectors: {
			builtin: "service"
		}
	}],
}, {
	group: "apps",
	resource: "statefulSet",
	subResources: [{
		group: "",
		resource: "pod",
		selectors: {
			ownerReference: true,
		},
	}],
}]
`,
			},
			expectedErr: "no matches for invalid",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	subResources: [{
		group: "apps",
		resource: "statefulSet",
		selectors: {
			ownerReference: true,
		},
	}],
	peerResources: [{
		group: "",
		resource: "configMap",
		selectors: {
			name: context.data.metadata.name,
		},
	}, {
		group: "",
		resource: "configMap",
		selectors: {
			namespace: "cm",
		},
	}, {
		group: "",
		resource: "configMap",
		selectors: {
			annotations: "anno": "value",
			labels: "label": "value2",
		},
	},  {
		group: "",
		resource: "configMap",
		selectors: {
			labels: "label": "value",
		},
	}, {
		group: "",
		resource: "service",
		selectors: {
			builtin: "service"
		}
	}],
}, {
	group: "apps",
	resource: "statefulSet",
	subResources: [{
		group: "",
		resource: "pod",
		selectors: {
			ownerReference: true,
		},
	}],
}]
`,
			},
			expected: []k8s.ResourceIdentifier{
				{
					Group:     "",
					Resource:  "configMap",
					Name:      "test-deploy",
					Namespace: "default",
				},
				{
					Group:     "",
					Resource:  "configMap",
					Name:      "cm1",
					Namespace: "cm",
				},
				{
					Group:     "",
					Resource:  "configMap",
					Name:      "cm2",
					Namespace: "default",
				},
				{
					Group:     "",
					Resource:  "configMap",
					Name:      "cm3",
					Namespace: "default",
				},
				{
					Group:     "",
					Resource:  "service",
					Name:      "test-svc",
					Namespace: "default",
				},
			},
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	group: "apps",
	resource: "deployment",
	peerResources: [{
		group: "core",
		resource: "configMap",
		selectors: {
			name: [
				for v in context.data.spec.template.spec.volumes if v.configMap != _|_ if v.configMap.name != _|_ {
					v.configMap.name
				}
			],
		},
	}],
}]
`,
			},
			expected: []k8s.ResourceIdentifier{
				{
					Group:     "core",
					Resource:  "configMap",
					Name:      "cm1",
					Namespace: "default",
				},
				{
					Group:     "core",
					Resource:  "configMap",
					Name:      "cm2",
					Namespace: "default",
				},
			},
		},
	}

	// Run Tests
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			r := require.New(t)
			peers, err := tc.rt.GetPeerResources(ctx, tc.resource)
			if tc.expectedErr != "" {
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)
			r.Equal(tc.expected, peers)
		})
	}
}
