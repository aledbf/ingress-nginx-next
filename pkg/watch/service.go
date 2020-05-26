package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/ingress-nginx-next/pkg/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Services struct {
	watcher *watcher

	references reference.ObjectRefMap
}

func NewServiceWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Services, error) {
	services := &Services{
		references: reference.NewObjectRefMap(),
	}
	w, err := NewWatcher("services", &corev1.Service{}, services.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go w.Start(stopCh)

	services.watcher = w
	return services, nil
}

func (sw *Services) Get(key types.NamespacedName) (*corev1.Service, error) {
	obj, err := sw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Service)
	return svc, nil
}

func (sw *Services) Add(ingress types.NamespacedName, services []types.NamespacedName) error {
	for _, configmap := range services {
		sw.references.Insert(ingress.String(), configmap.String())
	}

	return sw.watcher.Add(ingress, services)
}

func (sw *Services) RemoveReferencedBy(ingress types.NamespacedName) {
	key := ingress.String()
	if !sw.references.HasConsumer(key) {
		// there is no configmap references
		return
	}

	services := sw.references.ReferencedBy(key)
	for _, configmap := range services {
		//cw.watcher.remove(configmap)
		sw.references.Delete(configmap)
	}
}

func (sw *Services) isReferenced(key types.NamespacedName) bool {
	ctrl.Log.Info("IsReferenced", "from", "Services")
	references := sw.references.Reference(key.String())
	return len(references) > 1
}
