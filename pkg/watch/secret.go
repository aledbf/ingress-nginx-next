package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Secrets struct {
	watcher *watcher
}

func NewSecretWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Secrets, error) {
	secrets := &Secrets{}
	sw, err := NewWatcher("secrets", &corev1.Secret{}, secrets.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go sw.Start(stopCh)

	secrets.watcher = sw
	return secrets, nil
}

func (sw *Secrets) Get(key types.NamespacedName) (*corev1.Secret, error) {
	obj, err := sw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Secret)
	return svc, nil
}

func (sw *Secrets) Add(ingress types.NamespacedName, keys []types.NamespacedName) error {
	return sw.watcher.Add(ingress, keys)
}

func (sw *Secrets) RemoveReferencedBy(ingress types.NamespacedName) {
}

func (sw *Secrets) isReferenced(key types.NamespacedName) bool {
	ctrl.Log.Info("IsReferenced", "from", "Secrets")
	return true
}
