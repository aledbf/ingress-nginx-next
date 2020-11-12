package watch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Endpoints struct {
	watcher *watcher

	references reference.ObjectRefMap
}

func NewEndpointsWatcher(eventCh chan Event, ctx context.Context, mgr manager.Manager) (*Endpoints, error) {
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

func (sw *Endpoints) Get(key string) (*corev1.Endpoints, error) {
	obj, err := sw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Endpoints)
	return svc, nil
}

func (sw *Endpoints) Add(key string, endpoints []string) {
	for _, endpoint := range endpoints {
		sw.references.Insert(key, endpoint)
	}

	sw.watcher.Add(key, endpoints)
}

func (sw *Endpoints) RemoveReferencedBy(key string) {
	if !sw.references.HasConsumer(key) {
		// there is no endpoints references
		return
	}

	endpoints := sw.references.ReferencedBy(key)
	for _, endpoint := range endpoints {
		sw.watcher.remove(endpoint)
		sw.references.Delete(endpoint)
	}
}

func (sw *Endpoints) isReferenced(key string) bool {
	references := sw.references.Reference(key)
	return len(references) > 0
}
