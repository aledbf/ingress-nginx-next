package watch

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type Watcher interface {
	AddConfigmap(key types.NamespacedName) error

	AddService(key types.NamespacedName) error
	GetService(key types.NamespacedName) *ServiceWatcher
	AddServices(keys []types.NamespacedName) error

	RemoveConfigmap(key types.NamespacedName) error
	RemoveService(key types.NamespacedName) error
}

type ObjectWatcher struct {
	client kubernetes.Interface

	configmaps map[string]*configmapWatcher
	services   map[string]*ServiceWatcher

	Events chan Event

	stopCh chan struct{}
}

func NewObjectWatcher(events chan Event, stopCh <-chan struct{}, client kubernetes.Interface) *ObjectWatcher {
	return &ObjectWatcher{
		configmaps: make(map[string]*configmapWatcher),
		services:   make(map[string]*ServiceWatcher),

		client: client,

		Events: events,
	}
}

func (ow *ObjectWatcher) AddConfigmap(key types.NamespacedName) error {
	if _, exists := ow.configmaps[key.String()]; exists {
		return nil
	}

	cm, err := newConfigmapWatcher(key, ow.Events, ow.stopCh, ow.client)
	if err != nil {
		return err
	}

	ow.configmaps[key.String()] = &cm

	return nil
}

func (ow *ObjectWatcher) AddServices(keys []types.NamespacedName) error {
	for _, key := range keys {
		if _, exists := ow.services[key.String()]; exists {
			continue
		}

		svc, err := newServiceWatcher(key, ow.Events, ow.stopCh, ow.client)
		if err != nil {
			return err
		}

		ow.services[key.String()] = &svc
	}

	return nil
}

func (ow *ObjectWatcher) GetService(key types.NamespacedName) (ServiceWatcher, error) {
	if sw, exists := ow.services[key.String()]; exists {
		return *sw, nil
	}

	return nil, fmt.Errorf("there is no service %v", key)
}

func (ow *ObjectWatcher) RemoveConfigmap(key types.NamespacedName) error {
	if _, exists := ow.configmaps[key.String()]; !exists {
		return fmt.Errorf("")
	}

	delete(ow.configmaps, key.String())
	return nil
}
