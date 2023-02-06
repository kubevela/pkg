/*
Copyright 2021 The KubeVela Authors.

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

package patch

import (
	"encoding/json"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/k8s"
)

// PatchAction is the action for patch
type PatchAction struct {
	UpdateAnno            bool
	AnnoLastAppliedConfig string
	AnnoLastAppliedTime   string
}

// ThreeWayMergePatch creates a patch by computing a three way diff based on
// its current state, modified state, and last-applied-state recorded in the
// annotation.
func ThreeWayMergePatch(currentObj, modifiedObj runtime.Object, a *PatchAction) (client.Patch, error) {
	current, err := json.Marshal(currentObj)
	if err != nil {
		return nil, err
	}
	original := getOriginalConfiguration(currentObj, a.AnnoLastAppliedConfig)
	modified, err := getModifiedConfiguration(modifiedObj, a.UpdateAnno, a.AnnoLastAppliedConfig, a.AnnoLastAppliedTime)
	if err != nil {
		return nil, err
	}

	var patchType types.PatchType
	var patchData []byte
	var lookupPatchMeta strategicpatch.LookupPatchMeta

	versionedObject, err := scheme.Scheme.New(currentObj.GetObjectKind().GroupVersionKind())
	switch {
	case runtime.IsNotRegisteredError(err):
		// use JSONMergePatch for custom resources
		// because StrategicMergePatch doesn't support custom resources
		patchType = types.MergePatchType
		preconditions := []mergepatch.PreconditionFunc{
			mergepatch.RequireKeyUnchanged("apiVersion"),
			mergepatch.RequireKeyUnchanged("kind"),
			mergepatch.RequireMetadataKeyUnchanged("name")}
		patchData, err = jsonmergepatch.CreateThreeWayJSONMergePatch(original, modified, current, preconditions...)
		if err != nil {
			return nil, err
		}
	default:
		// use StrategicMergePatch for K8s built-in resources
		patchType = types.StrategicMergePatchType
		lookupPatchMeta, err = strategicpatch.NewPatchMetaFromStruct(versionedObject)
		if err != nil {
			return nil, err
		}
		patchData, err = strategicpatch.CreateThreeWayMergePatch(original, modified, current, lookupPatchMeta, true)
		if err != nil {
			return nil, err
		}
	}
	return client.RawPatch(patchType, patchData), nil
}

// addLastAppliedConfigAnnotation creates annotation recording current configuration as
// original configuration for latter use in computing a three way diff
func addLastAppliedConfigAnnotation(obj runtime.Object, annoAppliedConfig, annoAppliedTime string) error {
	config, err := getModifiedConfiguration(obj, false, annoAppliedConfig, annoAppliedTime)
	if err != nil {
		return err
	}

	return k8s.AddAnnotation(obj, annoAppliedConfig, string(config))
}

// getModifiedConfiguration serializes the object into byte stream.
// If `updateAnnotation` is true, it embeds the result as an annotation in the
// modified configuration.
func getModifiedConfiguration(obj runtime.Object, updateAnnotation bool, annoAppliedConfig, annoAppliedTime string) ([]byte, error) {
	if err := k8s.DeleteAnnotation(obj, annoAppliedConfig); err != nil {
		return nil, err
	}
	o := obj.DeepCopyObject()

	modified, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}

	if updateAnnotation {
		err := k8s.AddAnnotation(o, annoAppliedConfig, string(modified))
		if err != nil {
			return nil, err
		}
		modified, err = json.Marshal(o)
		if err != nil {
			return nil, err
		}
	}

	if err := k8s.AddAnnotation(obj, annoAppliedTime, time.Now().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	return modified, nil
}

// getOriginalConfiguration gets original configuration of the object
// form the annotation, or nil if no annotation found.
func getOriginalConfiguration(obj runtime.Object, anno string) []byte {
	original := k8s.GetAnnotation(obj, anno)
	return []byte(original)
}
