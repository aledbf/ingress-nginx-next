package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Endpoints struct {
	watcher *watcher
}

func NewEndpointsWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Endpoints, error) {
	ew, err := NewWatcher(&corev1.Endpoints{}, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go ew.Start(stopCh)

	return &Endpoints{
		watcher: ew,
	}, nil
}

func (ew *Endpoints) Get(key types.NamespacedName) (*corev1.Endpoints, error) {
	obj, err := ew.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Endpoints)
	return svc, nil
}

func (sw *Endpoints) Add(keys []types.NamespacedName) error {
	return sw.watcher.Add(keys)
}

func (sw *Endpoints) Remove(key types.NamespacedName) error {
	return sw.watcher.Remove(key)
}
