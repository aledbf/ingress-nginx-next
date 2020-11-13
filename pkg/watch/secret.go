package watch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Secrets struct {
	watcher Watcher

	references reference.ObjectRefMap
}

func NewSecretWatcher(ctx context.Context, eventCh chan Event, mgr manager.Manager) (*Secrets, error) {
	secrets := &Secrets{
		references: reference.NewObjectRefMap(),
	}

	partialMetadata := meta.AsPartialObjectMetadata(&corev1.Secret{})
	partialMetadata.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))

	w, err := NewWatcher("secrets", partialMetadata, secrets.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go w.Start(ctx)

	secrets.watcher = w
	return secrets, nil
}

func (sw *Secrets) Get(key string) (*corev1.Secret, error) {
	obj, err := sw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{}

	opm := obj.(*metav1.PartialObjectMetadata)
	opm.ObjectMeta.DeepCopyInto(&secret.ObjectMeta)

	return secret, nil
}

func (sw *Secrets) Add(key string, secrets []string) {
	for _, secret := range secrets {
		sw.references.Insert(key, secret)
	}

	sw.watcher.Add(key, secrets)
}

func (sw *Secrets) RemoveReferencedBy(key string) {
	if !sw.references.HasConsumer(key) {
		return
	}

	secrets := sw.references.ReferencedBy(key)
	for _, secret := range secrets {
		sw.watcher.Remove(secret)
		sw.references.Delete(secret)
	}
}

func (sw *Secrets) isReferenced(key string) bool {
	references := sw.references.Reference(key)
	return len(references) > 0
}
