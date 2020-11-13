package watch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	local_types "k8s.io/ingress-nginx-next/pkg/types"
)

type Services struct {
	watcher Watcher

	references reference.ObjectRefMap
}

func NewServiceWatcher(eventCh chan Event, mgr manager.Manager) (*Services, error) {
	services := &Services{
		references: reference.NewObjectRefMap(),
	}

	var err error
	services.watcher, err = NewWatcher("services", &corev1.Service{}, services.isReferenced, eventCh, mgr)
	if err != nil {
		return nil, err
	}

	return services, nil
}

func (sw *Services) Start(ctx context.Context) {
	//	sw.watcher.Start(ctx)
}

func (sw *Services) Get(key types.NamespacedName) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := sw.watcher.Get(context.Background(), key, svc)
	if err != nil {
		return nil, err
	}

	return svc, nil
}

func (sw *Services) Add(key types.NamespacedName, services []string) {
	for _, service := range services {
		sw.references.Insert(key, local_types.ParseNamespacedName(service))
	}

	sw.watcher.Add(key, services)
}

func (sw *Services) RemoveReferencedBy(key types.NamespacedName) {
	if !sw.references.HasConsumer(key) {
		return
	}

	services := sw.references.ReferencedBy(key)
	for _, service := range services {
		sw.references.Delete(service)
	}
}

func (sw *Services) isReferenced(key types.NamespacedName) bool {
	references := sw.references.Reference(key)
	return len(references) > 0
}
