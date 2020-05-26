package watch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Endpoints struct {
	watcher *watcher
}

func NewEndpointsWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Endpoints, error) {
	endpoints := &Endpoints{}
	ew, err := NewWatcher("endpoints", &corev1.Endpoints{}, endpoints.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	go ew.Start(stopCh)

	endpoints.watcher = ew
	return endpoints, nil
}

func (ew *Endpoints) Get(key types.NamespacedName) (*corev1.Endpoints, error) {
	obj, err := ew.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Endpoints)
	return svc, nil
}

func (ew *Endpoints) Add(ingress types.NamespacedName, keys []types.NamespacedName) error {
	return ew.watcher.Add(ingress, keys)
}

func (ew *Endpoints) RemoveReferencedBy(ingress types.NamespacedName) {
}

func (ew *Endpoints) isReferenced(key types.NamespacedName) bool {
	ctrl.Log.Info("IsReferenced", "from", "Endpoints")
	return true
}
