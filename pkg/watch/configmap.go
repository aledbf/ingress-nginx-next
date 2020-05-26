package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Configmaps struct {
	watcher *watcher
}

func NewConfigmapWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Configmaps, error) {
	sw, err := NewWatcher("configmaps", &corev1.ConfigMap{}, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go sw.Start(stopCh)

	return &Configmaps{
		watcher: sw,
	}, nil
}

func (sw *Configmaps) Get(key types.NamespacedName) (*corev1.ConfigMap, error) {
	obj, err := sw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.ConfigMap)
	return svc, nil
}

func (sw *Configmaps) Add(keys []types.NamespacedName) error {
	return sw.watcher.Add(keys)
}

func (sw *Configmaps) Remove(key types.NamespacedName) error {
	return sw.watcher.Remove(key)
}
