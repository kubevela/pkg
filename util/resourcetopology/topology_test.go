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
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "test-deploy",
		Namespace:  "default",
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
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "not-found",
				Namespace:  "default",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
}]
`,
			},
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	apiVersion: "apps/v1",
	kind: "Deployment",
	subResources: [{
		apiVersion: "a/b/c",
		kind: "StatefulSet",
		selectors: ownerReference: true
	}]
}]
`,
			},
			expectedErr: "unexpected",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	apiVersion: "apps/v1",
	kind: "Deployment",
	subResources: [{
		apiVersion: "apps/v1",
		kind: "StatefulSet",
		selectors: {
			namespace: context.data.metadata.namespace,
			ownerReference: true,
		},
	}],
}, {
	apiVersion: "apps/v1",
	kind: "StatefulSet",
	subResources: [{
		apiVersion: "a/b/c",
		kind: "Pod",
		selectors: {
			namespace: context.data.metadata.namespace,
			ownerReference: true,
		},
	}],
}]
`,
			},
			expectedErr: "unexpected",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	apiVersion: "apps/v1",
	kind: "Deployment",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
	subResources: [{
		apiVersion: "apps/v1",
		kind: "StatefulSet",
		selectors: {
			namespace: context.data.metadata.namespace,
			ownerReference: true,
		},
	}],
}, {
	apiVersion: "apps/v1",
	kind: "StatefulSet",
	subResources: [{
		apiVersion: "v1",
		kind: "Pod",
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
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "test-stateful",
						Namespace:  "default",
					},
					Children: []SubResource{
						{
							ResourceIdentifier: k8s.ResourceIdentifier{
								APIVersion: "v1",
								Kind:       "Pod",
								Name:       "test-pod",
								Namespace:  "default",
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
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "test-deploy",
		Namespace:  "default",
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
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "not-found",
				Namespace:  "default",
			},
			expectedErr: "not found",
		},
		{
			resource: k8s.ResourceIdentifier{
				APIVersion: "invalid",
				Kind:       "Deployment",
				Name:       "test-deploy",
				Namespace:  "default",
			},
			expectedErr: "no matches for kind",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	apiVersion: "apps/v1",
	kind: "Deployment",
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
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
	apiVersion: "apps/v1",
	kind: "Deployment"
}]
`,
			},
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	apiVersion: "apps/v1",
	kind: "Deployment"
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
	apiVersion: "apps/v1",
	kind: "Deployment",
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
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
	apiVersion: "apps/v1",
	kind: "Deployment",
	subResources: [{
		apiVersion: "a/b/c",
		kind: "StatefulSet",
		selectors: {
			ownerReference: true,
		},
	}],
	peerResources: [{
		apiVersion: "v1",
		kind: "Service",
		selectors: {
			builtin: "service"
		}
	}],
}, {
	apiVersion: "apps/v1",
	kind: "StatefulSet",
	subResources: [{
		apiVersion: "v1",
		kind: "Pod",
		selectors: {
			ownerReference: true,
		},
	}],
}]
`,
			},
			expectedErr: "unexpected",
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	apiVersion: "apps/v1",
	kind: "Deployment",
	subResources: [{
		apiVersion: "apps/v1",
		kind: "StatefulSet",
		selectors: {
			ownerReference: true,
		},
	}],
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
		selectors: {
			name: context.data.metadata.name,
		},
	}, {
		apiVersion: "v1",
		kind: "ConfigMap",
		selectors: {
			namespace: "cm",
		},
	}, {
		apiVersion: "v1",
		kind: "ConfigMap",
		selectors: {
			annotations: "anno": "value",
			labels: "label": "value2",
		},
	},  {
		apiVersion: "v1",
		kind: "ConfigMap",
		selectors: {
			labels: "label": "value",
		},
	}, {
		apiVersion: "v1",
		kind: "Service",
		selectors: {
			builtin: "service"
		}
	}],
}, {
	apiVersion: "apps/v1",
	kind: "StatefulSet",
	subResources: [{
		apiVersion: "v1",
		kind: "Pod",
		selectors: {
			ownerReference: true,
		},
	}],
}]
`,
			},
			expected: []k8s.ResourceIdentifier{
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test-deploy",
					Namespace:  "default",
				},
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "cm1",
					Namespace:  "cm",
				},
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "cm2",
					Namespace:  "default",
				},
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "cm3",
					Namespace:  "default",
				},
				{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "test-svc",
					Namespace:  "default",
				},
			},
		},
		{
			resource: defaultIdentifier,
			rt: &engine{
				ruleTemplate: `
rules: [{
	apiVersion: "apps/v1",
	kind: "Deployment",
	peerResources: [{
		apiVersion: "v1",
		kind: "ConfigMap",
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
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "cm1",
					Namespace:  "default",
				},
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "cm2",
					Namespace:  "default",
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
