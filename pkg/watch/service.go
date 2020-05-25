package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Services struct {
	watcher *watcher
}

func NewServiceWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Services, error) {
	sw, err := NewWatcher("services", &corev1.Service{}, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go sw.Start(stopCh)

	return &Services{
		watcher: sw,
	}, nil
}

func (sw *Services) Get(key types.NamespacedName) (*corev1.Service, error) {
	obj, err := sw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Service)
	return svc, nil
}

func (sw *Services) Add(keys []types.NamespacedName) error {
	return sw.watcher.Add(keys)
}

func (sw *Services) Remove(key types.NamespacedName) error {
	return sw.watcher.Remove(key)
}
