package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Secrets struct {
	watcher *watcher

	references reference.ObjectRefMap
}

func NewSecretWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Secrets, error) {
	secrets := &Secrets{
		references: reference.NewObjectRefMap(),
	}
	w, err := NewWatcher("secrets", &corev1.Secret{}, secrets.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go w.Start(stopCh)

	secrets.watcher = w
	return secrets, nil
}

func (sw *Secrets) Get(key types.NamespacedName) (*corev1.Secret, error) {
	obj, err := sw.watcher.Get(key.String())
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Secret)
	return svc, nil
}

func (sw *Secrets) Add(ingress types.NamespacedName, secrets []types.NamespacedName) error {
	for _, secret := range secrets {
		sw.references.Insert(ingress.String(), secret.String())
	}

	return sw.watcher.Add(ingress.String(), secrets)
}

func (sw *Secrets) RemoveReferencedBy(ingress types.NamespacedName) {
	key := ingress.String()
	if !sw.references.HasConsumer(key) {
		// there is no secret references
		return
	}

	secrets := sw.references.ReferencedBy(key)
	for _, secret := range secrets {
		sw.watcher.remove(secret)
		sw.references.Delete(secret)
	}
}

func (sw *Secrets) isReferenced(key string) bool {
	references := sw.references.Reference(key)
	return len(references) > 0
}
