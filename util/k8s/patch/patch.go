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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/k8s"
)

// PatchAction is the action for patch
type PatchAction struct {
	// UpdateAnno update the annotation of last-applied-configuration on modifiedObj
	UpdateAnno bool
	// AnnoLastAppliedConfig the annotation key for last-applied-configuration
	AnnoLastAppliedConfig string
	// AnnoLastAppliedTime the annotation key for last-applied-time
	AnnoLastAppliedTime string
}

// ThreeWayMergePatch creates a patch by computing a three way diff based on
// its current state, modified state, and last-applied-state recorded in the
// annotation.
func ThreeWayMergePatch(currentObj, modifiedObj runtime.Object, a *PatchAction) (client.Patch, error) {
	current, err := json.Marshal(currentObj)
	if err != nil {
		return nil, err
	}
	original := GetOriginalConfiguration(currentObj, a.AnnoLastAppliedConfig)
	modified, err := GetModifiedConfiguration(modifiedObj, a.AnnoLastAppliedConfig, a.AnnoLastAppliedTime)
	if err != nil {
		return nil, err
	}

	var patchType types.PatchType
	var patchData []byte

	versionedObject, err := clientgoscheme.Scheme.New(currentObj.GetObjectKind().GroupVersionKind())
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
	case err != nil:
		return nil, err
	default:
		// use StrategicMergePatch for K8s built-in resources
		patchType = types.StrategicMergePatchType
		lookupPatchMeta, err := strategicpatch.NewPatchMetaFromStruct(versionedObject)
		if err != nil {
			return nil, err
		}
		patchData, err = strategicpatch.CreateThreeWayMergePatch(original, modified, current, lookupPatchMeta, true)
		if err != nil {
			return nil, err
		}
	}
	if a.UpdateAnno && patchData != nil && string(patchData) != "{}" {
		_data := map[string]any{}
		if err = json.Unmarshal(patchData, &_data); err != nil {
			return nil, err
		}
		_ = unstructured.SetNestedField(_data, string(modified), "metadata", "annotations", a.AnnoLastAppliedConfig)
		_ = unstructured.SetNestedField(_data, time.Now().Format(time.RFC3339), "metadata", "annotations", a.AnnoLastAppliedTime)
		if patchData, err = json.Marshal(_data); err != nil {
			return nil, err
		}
	}
	return client.RawPatch(patchType, patchData), nil
}

// AddLastAppliedConfiguration add last-applied-configuration and last-applied-time annotation
func AddLastAppliedConfiguration(obj runtime.Object, annoAppliedConfig string, annoAppliedTime string) error {
	modified, err := GetModifiedConfiguration(obj, annoAppliedConfig, annoAppliedTime)
	if err != nil {
		return err
	}
	_ = k8s.AddAnnotation(obj, annoAppliedConfig, string(modified))
	_ = k8s.AddAnnotation(obj, annoAppliedTime, time.Now().Format(time.RFC3339))
	return nil
}

// GetModifiedConfiguration serializes the object into byte stream.
// If `updateAnnotation` is true, it embeds the result as an annotation in the
// modified configuration.
func GetModifiedConfiguration(obj runtime.Object, annoAppliedConfig string, annoAppliedTime string) ([]byte, error) {
	// copy the original one, remove last-applied-configuration and serialize it
	o := obj.DeepCopyObject()
	_ = k8s.DeleteAnnotation(o, annoAppliedConfig)
	_ = k8s.DeleteAnnotation(o, annoAppliedTime)
	modified, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	return modified, err
}

// GetOriginalConfiguration gets original configuration of the object
// form the annotation, or nil if no annotation found.
func GetOriginalConfiguration(obj runtime.Object, anno string) []byte {
	original := k8s.GetAnnotation(obj, anno)
	switch original {
	case "":
		return []byte(k8s.GetAnnotation(obj, corev1.LastAppliedConfigAnnotation))
	case "-", "skip":
		return []byte(``)
	default:
		return []byte(original)
	}
}
