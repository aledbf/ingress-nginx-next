package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Secrets struct {
	watcher *watcher
}

func NewSecretWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Secrets, error) {
	sw, err := NewWatcher("secrets", &corev1.Secret{}, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go sw.Start(stopCh)

	return &Secrets{
		watcher: sw,
	}, nil
}

func (sw *Secrets) Get(key types.NamespacedName) (*corev1.Secret, error) {
	obj, err := sw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Secret)
	return svc, nil
}

func (sw *Secrets) Add(keys []types.NamespacedName) error {
	return sw.watcher.Add(keys)
}
