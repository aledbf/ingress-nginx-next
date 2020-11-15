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
	"testing"

	"k8s.io/apimachinery/pkg/types"

	local_types "k8s.io/ingress-nginx-next/pkg/types"
)

func TestObjectRefMapOperations(t *testing.T) {
	orm := NewObjectRefMap()

	items := []struct {
		consumer types.NamespacedName
		ref      []types.NamespacedName
	}{
		{types.NamespacedName{Namespace: "ns", Name: "ingress1"}, []types.NamespacedName{{Namespace: "ns", Name: "tls1"}}},
		{types.NamespacedName{Namespace: "ns", Name: "ingress2"}, []types.NamespacedName{{Namespace: "ns", Name: "tls1"}, {Namespace: "ns", Name: "tls2"}}},
		{types.NamespacedName{Namespace: "ns", Name: "ingress3"}, []types.NamespacedName{{Namespace: "ns", Name: "tls1"}, {Namespace: "ns", Name: "tls2"}, {Namespace: "ns", Name: "tls3"}}},
	}

	if orm.Has(local_types.ParseNamespacedName("ns/tls1")) {
		t.Error(`Expected false (got true)`)
	}

	// populate map with test data
	for _, i := range items {
		orm.Insert(i.consumer, i.ref...)
	}
	if l := orm.Len(); l != 3 {
		t.Fatalf("Expected 3 referenced objects (got %d)", l)
	}

	if !orm.Has(local_types.ParseNamespacedName("ns/tls1")) {
		t.Error(`Expected true (got false)`)
	}

	// add already existing item
	orm.Insert(local_types.ParseNamespacedName("ns/ingress1"), local_types.ParseNamespacedName("ns/tls1"))
	if l := len(orm.ReferencedBy(local_types.ParseNamespacedName("ns/ingress1"))); l != 1 {
		t.Error("Expected existing item not to be added again")
	}

	// find consumer by name
	if !orm.HasConsumer(local_types.ParseNamespacedName("ns/ingress1")) {
		t.Error(`Expected the "ns/ingress1" consumer to exist in the map`)
	}

	// count references to object
	if l := len(orm.Reference(local_types.ParseNamespacedName("ns/tls1"))); l != 3 {
		t.Errorf(`Expected "ns/tls1" to be referenced by 3 objects (got %d)`, l)
	}

	// delete consumer
	orm.Delete(local_types.ParseNamespacedName("ns/ingress3"))
	if l := orm.Len(); l != 2 {
		t.Errorf(`Expected 2 referenced objects (got %d)`, l)
	}
	if orm.Has(local_types.ParseNamespacedName("ns/tls3")) {
		t.Error(`Expected "ns/tls3" not to be referenced`)
	}
}
