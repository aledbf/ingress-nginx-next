package watch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	local_types "k8s.io/ingress-nginx-next/pkg/types"
)

type Configmaps struct {
	watcher Watcher

	references reference.ObjectRefMap
}

func NewConfigmapWatcher(ctx context.Context, eventCh chan Event, mgr manager.Manager) (*Configmaps, error) {
	configmaps := &Configmaps{
		references: reference.NewObjectRefMap(),
	}
	w, err := NewWatcher("configmaps", &corev1.ConfigMap{}, configmaps.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go w.Start(ctx)

	configmaps.watcher = w
	return configmaps, nil
}

func (cw *Configmaps) Get(key types.NamespacedName) (*corev1.ConfigMap, error) {
	cmap := &corev1.ConfigMap{}
	err := cw.watcher.Get(context.Background(), key, cmap)
	if err != nil {
		return nil, err
	}

	return cmap, nil
}

func (cw *Configmaps) Add(ingress types.NamespacedName, configmaps []string) {
	for _, configmap := range configmaps {
		cw.references.Insert(ingress, local_types.ParseNamespacedName(configmap))
	}

	cw.watcher.Add(ingress, configmaps)
}

func (cw *Configmaps) RemoveReferencedBy(ingress types.NamespacedName) {
	if !cw.references.HasConsumer(ingress) {
		return
	}

	configmaps := cw.references.ReferencedBy(ingress)
	for _, configmap := range configmaps {
		cw.watcher.Remove(configmap)
		cw.references.Delete(configmap)
	}
}

func (cw *Configmaps) isReferenced(key types.NamespacedName) bool {
	references := cw.references.Reference(key)
	return len(references) > 0
}
