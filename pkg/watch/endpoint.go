package watch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	local_types "k8s.io/ingress-nginx-next/pkg/types"
)

type Endpoints struct {
	watcher Watcher

	references reference.ObjectRefMap
}

func NewEndpointsWatcher(ctx context.Context, eventCh chan Event, mgr manager.Manager) (*Endpoints, error) {
	endpoints := &Endpoints{
		references: reference.NewObjectRefMap(),
	}
	w, err := NewWatcher("endpoints", &corev1.Endpoints{}, endpoints.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go w.Start(ctx)

	endpoints.watcher = w
	return endpoints, nil
}

func (sw *Endpoints) Get(key types.NamespacedName) (*corev1.Endpoints, error) {
	endpoints := &corev1.Endpoints{}
	err := sw.watcher.Get(context.Background(), key, endpoints)
	if err != nil {
		return nil, err
	}

	return endpoints, nil
}

func (sw *Endpoints) Add(key types.NamespacedName, endpoints []string) {
	for _, endpoint := range endpoints {
		sw.references.Insert(key, local_types.ParseNamespacedName(endpoint))
	}

	sw.watcher.Add(key, endpoints)
}

func (sw *Endpoints) RemoveReferencedBy(key types.NamespacedName) {
	if !sw.references.HasConsumer(key) {
		return
	}

	endpoints := sw.references.ReferencedBy(key)
	for _, endpoint := range endpoints {
		sw.watcher.Remove(endpoint)
		sw.references.Delete(endpoint)
	}
}

func (sw *Endpoints) isReferenced(key types.NamespacedName) bool {
	references := sw.references.Reference(key)
	return len(references) > 0
}
