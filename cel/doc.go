/*
Copyright 2026 The KubeVela Authors.

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

// Package cel evaluates CEL expressions embedded in properties with $( ).
//
// It knows no roots by name. A host declares the roots its expressions may read
// and supplies a resolver for each, which answers every read before anything is
// evaluated, so no expression performs I/O.
//
// A tree is parsed once into a Plan, which serves each stage that needs it:
// Reads for dependencies and status, Check for admission, and Eval to resolve
// and evaluate.
package cel
