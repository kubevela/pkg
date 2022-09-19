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

package object

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UnknownObject is an object that implements runtime.Object and client.Object
// but does not register itself to any schema
type UnknownObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Chan chan int
}

// DeepCopyObject .
func (in *UnknownObject) DeepCopyObject() runtime.Object {
	out := &UnknownObject{TypeMeta: in.TypeMeta}
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	return out
}

// UnknownObjectList is a list of objects that implements client.ObjectList
// but does not register itself to any schema
type UnknownObjectList struct {
	metav1.TypeMeta
	metav1.ListMeta
}

// DeepCopyObject .
func (in *UnknownObjectList) DeepCopyObject() runtime.Object {
	out := &UnknownObjectList{TypeMeta: in.TypeMeta}
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	return out
}

var _ runtime.Object = &UnknownObject{}
var _ client.Object = &UnknownObject{}
var _ client.ObjectList = &UnknownObjectList{}
