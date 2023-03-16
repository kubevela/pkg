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
	"reflect"
	"strings"

	"cuelang.org/go/cue"
	"github.com/pkg/errors"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/slices"
)

// SubResource .
type SubResource struct {
	k8s.ResourceIdentifier
	Children []SubResource `json:"children"`
}

// ResourceSelector .
type ResourceSelector struct {
	apiVersion string
	kind       string
	namespace  string
	name       string
	builtin    string
	filters    filterSelector
}

type filterSelector struct {
	annotations    map[string]string
	listOptions    []client.ListOption
	ownerReference bool
}

type engine struct {
	ruleTemplate string
	rules        map[string]cue.Value
	cache        map[string][]k8s.ResourceIdentifier
}

// Engine .
type Engine interface {
	GetSubResources(ctx context.Context, resource k8s.ResourceIdentifier) ([]SubResource, error)
	GetPeerResources(ctx context.Context, resource k8s.ResourceIdentifier) ([]k8s.ResourceIdentifier, error)
}

const (
	rulesKey         = "rules"
	subResourcesKey  = "subResources"
	peerResourcesKey = "peerResources"
	selectorsKey     = "selectors"

	nameSelectorKey           = "name"
	namespaceSelectorKey      = "namespace"
	builtinSelectorKey        = "builtin"
	annotationsSelectorKey    = "annotations"
	labelsSelectorKey         = "labels"
	ownerReferenceSelectorKey = "ownerReference"

	builtinRuleService = "service"
	builtinRuleIngress = "ingress"
)

// New .
func New(rules string) Engine {
	return &engine{
		ruleTemplate: rules,
		rules:        make(map[string]cue.Value),
	}
}

// GetSubResources get sub resources of given resource
func (r *engine) GetSubResources(ctx context.Context, resource k8s.ResourceIdentifier) ([]SubResource, error) {
	r.cache = make(map[string][]k8s.ResourceIdentifier)
	un, err := k8s.GetUnstructuredFromResource(ctx, resource)
	if err != nil {
		return nil, err
	}
	v, err := cuex.DefaultCompiler.Get().CompileStringWithOptions(ctx, r.ruleTemplate, cuex.WithExtraData("context", map[string]interface{}{
		"data": un,
	}))
	if err != nil {
		return nil, err
	}
	if v.Err() != nil {
		return nil, v.Err()
	}
	return r.getSubResources(ctx, v, resource)
}

func (r *engine) getSubResources(ctx context.Context, v cue.Value, resource k8s.ResourceIdentifier) ([]SubResource, error) {
	subResources := make([]SubResource, 0)
	rule, err := r.getRuleForResource(ctx, v, resource)
	if err != nil && !strings.Contains(err.Error(), "no rules found") {
		return nil, err
	}
	subs := rule.LookupPath(cue.ParsePath(subResourcesKey))
	if !subs.Exists() {
		return nil, nil
	}
	iter, err := subs.List()
	if err != nil {
		return nil, errors.Wrap(err, "subResources should be a list")
	}
	for iter.Next() {
		items, err := r.getResourcesWithSelector(ctx, iter.Value(), resource)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			children, err := r.getSubResources(ctx, v, item)
			if err != nil {
				return nil, err
			}
			subResources = append(subResources, SubResource{
				ResourceIdentifier: item,
				Children:           children,
			})
		}
	}
	return subResources, nil
}

func (r *engine) getResourceIdentifierWithValue(v cue.Value) (*k8s.ResourceIdentifier, error) {
	re := &k8s.ResourceIdentifier{}
	if err := v.Decode(re); err != nil {
		return nil, err
	}
	gvk, err := k8s.GetGVKFromResource(*re)
	if err != nil {
		return nil, err
	}
	apiVersion, kind := gvk.ToAPIVersionAndKind()
	re.APIVersion = apiVersion
	re.Kind = kind
	return re, nil
}

func (r *engine) getRuleForResource(ctx context.Context, v cue.Value, resource k8s.ResourceIdentifier) (cue.Value, error) {
	if len(r.rules) == 0 {
		r.rules = make(map[string]cue.Value)
		v = v.LookupPath(cue.ParsePath(rulesKey))
		if !v.Exists() {
			return cue.Value{}, fmt.Errorf("no rules found")
		}
		iter, err := v.List()
		if err != nil {
			return cue.Value{}, errors.Wrap(err, "rules should be a list")
		}
		for iter.Next() {
			re, err := r.getResourceIdentifierWithValue(iter.Value())
			if err != nil {
				return cue.Value{}, err
			}
			r.rules[fmt.Sprintf("%s/%s", re.APIVersion, re.Kind)] = iter.Value()
		}
	}
	if rule, ok := r.rules[fmt.Sprintf("%s/%s", resource.APIVersion, resource.Kind)]; ok {
		return rule, nil
	}
	return cue.Value{}, fmt.Errorf("no rules found for resource %s/%s", resource.APIVersion, resource.Kind)
}

// GetPeerResources get peer resources of given resource
func (r *engine) GetPeerResources(ctx context.Context, resource k8s.ResourceIdentifier) ([]k8s.ResourceIdentifier, error) {
	r.cache = make(map[string][]k8s.ResourceIdentifier)
	un, err := k8s.GetUnstructuredFromResource(ctx, resource)
	if err != nil {
		return nil, err
	}

	v, err := cuex.DefaultCompiler.Get().CompileStringWithOptions(ctx, r.ruleTemplate, cuex.WithExtraData("context", map[string]interface{}{
		"data": un,
	}))
	if err != nil {
		return nil, err
	}
	if v.Err() != nil {
		return nil, v.Err()
	}
	rule, err := r.getRuleForResource(ctx, v, resource)
	if err != nil {
		return nil, err
	}

	return r.getPeerResources(ctx, rule, resource)
}

func (r *engine) getPeerResources(ctx context.Context, rule cue.Value, resource k8s.ResourceIdentifier) ([]k8s.ResourceIdentifier, error) {
	peer := rule.LookupPath(cue.ParsePath(peerResourcesKey))
	if !peer.Exists() {
		return nil, nil
	}
	iter, err := peer.List()
	if err != nil {
		return nil, errors.Wrap(err, "peerResources should be a list")
	}
	peerResources := make([]k8s.ResourceIdentifier, 0)
	for iter.Next() {
		items, err := r.getResourcesWithSelector(ctx, iter.Value(), resource)
		if err != nil {
			return nil, err
		}
		peerResources = append(peerResources, items...)
	}
	return peerResources, nil
}

func (r *engine) getResourcesWithSelector(ctx context.Context, v cue.Value, resource k8s.ResourceIdentifier) ([]k8s.ResourceIdentifier, error) {
	base, err := r.getResourceIdentifierWithValue(v)
	if err != nil {
		return nil, err
	}
	selVal := v.LookupPath(cue.ParsePath(selectorsKey))
	if !selVal.Exists() {
		return nil, fmt.Errorf("selectors are required")
	}
	iter, err := selVal.Fields()
	if err != nil {
		return nil, err
	}
	resources := make([]k8s.ResourceIdentifier, 0)
	selector := ResourceSelector{
		apiVersion: base.APIVersion,
		kind:       base.Kind,
		namespace:  resource.Namespace,
	}
	selectByName := false
	names := make([]string, 0)
	for iter.Next() {
		switch iter.Label() {
		case builtinSelectorKey:
			typ, err := iter.Value().String()
			if err != nil {
				return nil, err
			}
			return r.handleBuiltInRules(ctx, typ, v, resource)
		case nameSelectorKey:
			selectByName = true
			nameVal := iter.Value()
			switch nameVal.Kind() {
			case cue.StringKind:
				name, _ := nameVal.String()
				names = append(names, name)
			default:
				err := nameVal.Decode(&names)
				if err != nil {
					return nil, err
				}
			}
		case namespaceSelectorKey:
			ns, err := iter.Value().String()
			if err != nil {
				return nil, err
			}
			selector.namespace = ns
			selector.filters.listOptions = append(selector.filters.listOptions, client.InNamespace(ns))
		case labelsSelectorKey:
			labels := make(map[string]string)
			if err := iter.Value().Decode(&labels); err == nil {
				selector.filters.listOptions = append(selector.filters.listOptions, client.MatchingLabels(labels))
			}
		case annotationsSelectorKey:
			_ = iter.Value().Decode(&selector.filters.annotations)
		case ownerReferenceSelectorKey:
			if b, err := iter.Value().Bool(); err == nil {
				selector.filters.ownerReference = b
				selector.filters.listOptions = append(selector.filters.listOptions, client.InNamespace(resource.Namespace))
			}
		default:
			return nil, fmt.Errorf("unsupported selector %s", iter.Label())
		}
	}

	switch {
	case selectByName:
		for _, name := range names {
			resources = append(resources, k8s.ResourceIdentifier{
				APIVersion: selector.apiVersion,
				Kind:       selector.kind,
				Namespace:  selector.namespace,
				Name:       name,
			})
		}
	default:
		result, err := listResources(ctx, selector, resource)
		if err != nil {
			return nil, err
		}
		for _, item := range result {
			resources = append(resources, k8s.ResourceIdentifier{
				APIVersion: selector.apiVersion,
				Kind:       selector.kind,
				Namespace:  item.GetNamespace(),
				Name:       item.GetName(),
			})
		}
	}
	return resources, nil
}

func (r *engine) handleBuiltInRules(ctx context.Context, typ string, v cue.Value, resource k8s.ResourceIdentifier) ([]k8s.ResourceIdentifier, error) {
	switch strings.ToLower(typ) {
	case builtinRuleService:
		return r.handleBuiltInRulesForService(ctx, v, resource)
	case builtinRuleIngress:
		return r.handleBuiltInRulesForIngress(ctx, v, resource)
	default:
		return nil, fmt.Errorf("unsupported built-in rule %s", typ)
	}
}

func (r *engine) getMatchResourceFromSubs(sub SubResource, apiVersion, kind string) []k8s.ResourceIdentifier {
	result := make([]k8s.ResourceIdentifier, 0)
	if sub.ResourceIdentifier.APIVersion == apiVersion && sub.ResourceIdentifier.Kind == kind {
		result = append(result, sub.ResourceIdentifier)
	}
	for _, child := range sub.Children {
		result = append(result, r.getMatchResourceFromSubs(child, apiVersion, kind)...)
	}
	return result
}

func (r *engine) handleBuiltInRulesForIngress(ctx context.Context, v cue.Value, resource k8s.ResourceIdentifier) ([]k8s.ResourceIdentifier, error) {
	var err error
	services, ok := r.cache[builtinRuleService]
	if !ok {
		services, err = r.handleBuiltInRulesForService(ctx, v, resource)
		if err != nil {
			return nil, err
		}
	}
	// get service endpoints and compare with pods
	ingressList := &networkingv1.IngressList{}
	if err = singleton.KubeClient.Get().List(ctx, ingressList, client.InNamespace(resource.Namespace)); err != nil {
		return nil, err
	}
	ingress := []k8s.ResourceIdentifier{}
	for _, item := range ingressList.Items {
		for _, rule := range item.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, p := range rule.HTTP.Paths {
				if p.Backend.Service == nil {
					continue
				}
				if slices.Contains(services, k8s.ResourceIdentifier{
					Name:       p.Backend.Service.Name,
					Namespace:  item.Namespace,
					APIVersion: "v1",
					Kind:       "Service",
				}) {
					ingress = append(ingress, k8s.ResourceIdentifier{
						APIVersion: "networking.k8s.io/v1",
						Kind:       "Ingress",
						Name:       item.Name,
						Namespace:  item.Namespace,
					})
				}
			}
		}
	}
	return ingress, nil
}

func (r *engine) handleBuiltInRulesForService(ctx context.Context, v cue.Value, resource k8s.ResourceIdentifier) ([]k8s.ResourceIdentifier, error) {
	if services, ok := r.cache[builtinRuleService]; ok {
		return services, nil
	}
	subs, err := r.getSubResources(ctx, v, resource)
	if err != nil {
		return nil, err
	}
	pods := make([]k8s.ResourceIdentifier, 0)
	for _, sub := range subs {
		pods = append(pods, r.getMatchResourceFromSubs(sub, "v1", "Pod")...)
	}
	// get service endpoints and compare with pods
	es := &discoveryv1.EndpointSliceList{}
	if err = singleton.KubeClient.Get().List(ctx, es, client.InNamespace(resource.Namespace)); err != nil {
		return nil, err
	}
	service := []k8s.ResourceIdentifier{}
	for _, e := range es.Items {
		for _, s := range e.Endpoints {
			if s.TargetRef == nil {
				continue
			}
			if slices.Contains(pods, k8s.ResourceIdentifier{
				Name:       s.TargetRef.Name,
				Namespace:  s.TargetRef.Namespace,
				APIVersion: "v1",
				Kind:       "Pod",
			}) {
				service = append(service, k8s.ResourceIdentifier{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       e.OwnerReferences[0].Name,
					Namespace:  resource.Namespace,
				})
			}
		}
	}
	r.cache[builtinRuleService] = service
	return service, nil
}

func listResources(ctx context.Context, selector ResourceSelector, relation k8s.ResourceIdentifier) ([]unstructured.Unstructured, error) {
	cli := singleton.KubeClient.Get()
	gvk, err := k8s.GetGVKFromResource(k8s.ResourceIdentifier{
		APIVersion: selector.apiVersion,
		Kind:       selector.kind,
	})
	if err != nil {
		return nil, err
	}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	if err := cli.List(ctx, list, selector.filters.listOptions...); err != nil {
		return nil, err
	}
	itemMap := make(map[string]unstructured.Unstructured)
	for _, un := range list.Items {
		itemMap[fmt.Sprintf("%s/%s/%s", un.GetKind(), un.GetNamespace(), un.GetName())] = un
	}
	for _, un := range list.Items {
		if len(selector.filters.annotations) > 0 {
			if !reflect.DeepEqual(un.GetAnnotations(), selector.filters.annotations) {
				delete(itemMap, fmt.Sprintf("%s/%s/%s", un.GetKind(), un.GetNamespace(), un.GetName()))
			}
		}
		if selector.filters.ownerReference {
			for _, ref := range un.GetOwnerReferences() {
				if !reflect.DeepEqual(k8s.ResourceIdentifier{
					APIVersion: ref.APIVersion,
					Kind:       ref.Kind,
					Name:       ref.Name,
					Namespace:  un.GetNamespace(),
				}, relation) {
					delete(itemMap, fmt.Sprintf("%s/%s/%s", un.GetKind(), un.GetNamespace(), un.GetName()))
				}
			}
		}
	}
	filtered := make([]unstructured.Unstructured, 0)
	for _, un := range itemMap {
		filtered = append(filtered, un)
	}
	return filtered, nil
}
