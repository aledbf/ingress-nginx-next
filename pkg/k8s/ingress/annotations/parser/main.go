/*
Copyright 2015 The Kubernetes Authors.

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

package parser

import (
	networking "k8s.io/api/networking/v1beta1"
)

var (
	// AnnotationsPrefix defines the common prefix used in the nginx ingress controller
	AnnotationsPrefix = "nginx.ingress.kubernetes.io"
)

// IngressAnnotation has a method to parse annotations located in Ingress
type IngressAnnotation interface {
	Parse(ing *networking.Ingress) (interface{}, error)
}
