package watch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Configmaps struct {
	watcher *watcher

	references reference.ObjectRefMap
}

func NewConfigmapWatcher(eventCh chan Event, stopCh context.Context, mgr manager.Manager) (*Configmaps, error) {
	configmaps := &Configmaps{
		references: reference.NewObjectRefMap(),
	}
	w, err := NewWatcher("configmaps", &corev1.ConfigMap{}, configmaps.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go w.Start(stopCh)

	configmaps.watcher = w
	return configmaps, nil
}

func (cw *Configmaps) Get(key string) (*corev1.ConfigMap, error) {
	obj, err := cw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.ConfigMap)
	return svc, nil
}

func (cw *Configmaps) Add(ingress string, configmaps []string) {
	for _, configmap := range configmaps {
		cw.references.Insert(ingress, configmap)
	}

	cw.watcher.Add(ingress, configmaps)
}

func (cw *Configmaps) RemoveReferencedBy(ingress string) {
	if !cw.references.HasConsumer(ingress) {
		// there is no configmap references
		return
	}

	configmaps := cw.references.ReferencedBy(ingress)
	for _, configmap := range configmaps {
		//cw.watcher.remove(configmap)
		cw.references.Delete(configmap)
	}
}

func (cw *Configmaps) isReferenced(key string) bool {
	references := cw.references.Reference(key)
	return len(references) > 0
}
