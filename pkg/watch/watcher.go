package watch

import (
	"fmt"
	"sync"

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

	configmaps map[string]*ConfigmapWatcher
	services   map[string]*ServiceWatcher

	Events chan Event

	stopCh chan struct{}

	configmapMu *sync.RWMutex
	serviceMu   *sync.RWMutex
}

func NewObjectWatcher(events chan Event, stopCh <-chan struct{}, client kubernetes.Interface) *ObjectWatcher {
	return &ObjectWatcher{
		configmaps: make(map[string]*ConfigmapWatcher),
		services:   make(map[string]*ServiceWatcher),

		configmapMu: &sync.RWMutex{},
		serviceMu:   &sync.RWMutex{},

		client: client,

		Events: events,
	}
}

func (ow *ObjectWatcher) AddConfigmap(key types.NamespacedName) error {
	ow.configmapMu.Lock()
	defer ow.configmapMu.Unlock()

	if _, exists := ow.configmaps[key.String()]; exists {
		return nil
	}

	cm := watchConfigmap(key, ow.Events, ow.stopCh, ow.client)
	ow.configmaps[key.String()] = &cm

	return nil
}

func (ow *ObjectWatcher) AddServices(keys []types.NamespacedName) error {
	ow.serviceMu.Lock()
	defer ow.serviceMu.Unlock()

	for _, key := range keys {
		if _, exists := ow.services[key.String()]; exists {
			continue
		}

		svc := newServiceWatcher(key, ow.Events, ow.stopCh, ow.client)
		ow.services[key.String()] = &svc
	}

	return nil
}

func (ow *ObjectWatcher) GetService(key types.NamespacedName) (ServiceWatcher, error) {
	ow.serviceMu.RLock()
	defer ow.serviceMu.RUnlock()

	if sw, exists := ow.services[key.String()]; exists {
		return *sw, nil
	}

	return nil, fmt.Errorf("service %v does not exists", key)
}

func (ow *ObjectWatcher) RemoveConfigmap(key types.NamespacedName) error {
	ow.configmapMu.Lock()
	defer ow.configmapMu.Unlock()

	if _, exists := ow.configmaps[key.String()]; !exists {
		return fmt.Errorf("configmap %v does not exists", key.String())
	}

	delete(ow.configmaps, key.String())
	return nil
}

func (ow *ObjectWatcher) RemoveService(key types.NamespacedName) error {
	ow.serviceMu.Lock()
	defer ow.serviceMu.Unlock()

	if _, exists := ow.services[key.String()]; !exists {
		return fmt.Errorf("configmap %v does not exists", key.String())
	}

	delete(ow.services, key.String())

	return nil
}
