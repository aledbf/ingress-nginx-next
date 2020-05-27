package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Endpoints struct {
	watcher *watcher

	references reference.ObjectRefMap
}

func NewEndpointsWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Endpoints, error) {
	endpoints := &Endpoints{
		references: reference.NewObjectRefMap(),
	}
	w, err := NewWatcher("endpoints", &corev1.Endpoints{}, endpoints.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go w.Start(stopCh)

	endpoints.watcher = w
	return endpoints, nil
}

func (sw *Endpoints) Get(key types.NamespacedName) (*corev1.Endpoints, error) {
	obj, err := sw.watcher.Get(key.String())
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Endpoints)
	return svc, nil
}

func (sw *Endpoints) Add(ingress types.NamespacedName, endpoints []types.NamespacedName) error {
	for _, endpoint := range endpoints {
		sw.references.Insert(ingress.String(), endpoint.String())
	}

	return sw.watcher.Add(ingress.String(), endpoints)
}

func (sw *Endpoints) RemoveReferencedBy(ingress types.NamespacedName) {
	key := ingress.String()
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
