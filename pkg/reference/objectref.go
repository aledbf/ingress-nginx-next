/*
Copyright 2018 The Kubernetes Authors.

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

package reference

import (
	"encoding/json"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	local_types "k8s.io/ingress-nginx-next/pkg/types"
)

// ObjectRefMap is a map of references from object(s) to object (1:n). It is
// used to keep track of which data objects (Secrets) are used within Ingress
// objects.
type ObjectRefMap interface {
	Insert(consumer types.NamespacedName, ref ...types.NamespacedName)
	Delete(consumer types.NamespacedName)
	Len() int
	Has(ref types.NamespacedName) bool
	HasConsumer(consumer types.NamespacedName) bool
	Reference(ref types.NamespacedName) []types.NamespacedName
	ReferencedBy(consumer types.NamespacedName) []types.NamespacedName
}

type objectRefMap struct {
	sync.Mutex
	v map[types.NamespacedName]sets.String
}

// NewObjectRefMap returns a new ObjectRefMap.
func NewObjectRefMap() ObjectRefMap {
	return &objectRefMap{
		v: make(map[types.NamespacedName]sets.String),
	}
}

// Insert adds a consumer to one or more referenced objects.
func (o *objectRefMap) Insert(consumer types.NamespacedName, ref ...types.NamespacedName) {
	o.Lock()
	defer o.Unlock()

	for _, r := range ref {
		if _, ok := o.v[r]; !ok {
			o.v[r] = sets.NewString(consumer.String())
			continue
		}
		o.v[r].Insert(consumer.String())
	}
}

// Delete deletes a consumer from all referenced objects.
func (o *objectRefMap) Delete(consumer types.NamespacedName) {
	o.Lock()
	defer o.Unlock()

	for ref, consumers := range o.v {
		consumers.Delete(consumer.String())
		if consumers.Len() == 0 {
			delete(o.v, ref)
		}
	}
}

// Len returns the count of referenced objects.
func (o *objectRefMap) Len() int {
	return len(o.v)
}

// Has returns whether the given object is referenced by any other object.
func (o *objectRefMap) Has(ref types.NamespacedName) bool {
	o.Lock()
	defer o.Unlock()

	if _, ok := o.v[ref]; ok {
		return true
	}
	return false
}

// HasConsumer returns whether the store contains the given consumer.
func (o *objectRefMap) HasConsumer(consumer types.NamespacedName) bool {
	o.Lock()
	defer o.Unlock()

	for _, consumers := range o.v {
		if consumers.Has(consumer.String()) {
			return true
		}
	}

	return false
}

// Reference returns all objects referencing the given object.
func (o *objectRefMap) Reference(ref types.NamespacedName) []types.NamespacedName {
	o.Lock()
	defer o.Unlock()

	consumers, ok := o.v[ref]
	if !ok {
		return make([]types.NamespacedName, 0)
	}

	return toNamespacedNameList(consumers.List())
}

// ReferencedBy returns all objects referenced by the given object.
func (o *objectRefMap) ReferencedBy(consumer types.NamespacedName) []types.NamespacedName {
	o.Lock()
	defer o.Unlock()

	refs := make([]types.NamespacedName, 0)
	for ref, consumers := range o.v {
		if consumers.Has(consumer.String()) {
			refs = append(refs, ref)
		}
	}

	return refs
}

func (o *objectRefMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.v)
}

func toNamespacedNameList(items []string) []types.NamespacedName {
	namespacedNames := []types.NamespacedName{}
	for _, item := range items {
		namespacedNames = append(namespacedNames, local_types.ParseNamespacedName(item))
	}

	return namespacedNames
}
