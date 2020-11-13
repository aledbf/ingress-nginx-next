package watch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/ingress-nginx-next/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
	sw.watcher.Start(ctx)
}

func (sw *Services) Get(key string) (*corev1.Service, error) {
	obj, err := sw.watcher.Get(key)
	if err != nil {
		return nil, err
	}

	svc := obj.(*corev1.Service)
	return svc, nil
}

func (sw *Services) Add(key string, services []string) {
	for _, service := range services {
		sw.references.Insert(key, service)
	}

	sw.watcher.Add(key, services)
}

func (sw *Services) RemoveReferencedBy(key string) {
	if !sw.references.HasConsumer(key) {
		return
	}

	services := sw.references.ReferencedBy(key)
	for _, service := range services {
		sw.watcher.Remove(service)
		sw.references.Delete(service)
	}
}

func (sw *Services) isReferenced(key string) bool {
	references := sw.references.Reference(key)
	return len(references) > 0
}
