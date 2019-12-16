// Package watch TODO...
package watch

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

type Event struct {
	metav1.TypeMeta

	NamespacedName types.NamespacedName
	Type           EventType
}

type EventType string

const (
	AddEvent    EventType = "Add"
	RemoveEvent EventType = "Remove"
	UpdateEvent EventType = "Update"
)

type Watcher interface {
	AddConfigmap(key types.NamespacedName) error
	AddService(key types.NamespacedName) error
	AddServices(keys []types.NamespacedName) error

	RemoveConfigmap(key types.NamespacedName) error
	RemoveService(key types.NamespacedName) error
}

type ObjectWatcher struct {
	client kubernetes.Interface

	configmaps map[types.NamespacedName]*configmapWatcher
	services   map[types.NamespacedName]*serviceWatcher

	Events chan Event

	stopCh chan struct{}
}

func NewObjectWatcher(events chan Event, stopCh chan struct{}, client kubernetes.Interface) *ObjectWatcher {
	return &ObjectWatcher{
		configmaps: make(map[types.NamespacedName]*configmapWatcher),
		services:   make(map[types.NamespacedName]*serviceWatcher),

		client: client,

		Events: events,
	}
}

func (ow *ObjectWatcher) AddConfigmap(key types.NamespacedName) error {
	if _, exists := ow.configmaps[key]; exists {
		return nil
	}

	cm, err := newConfigmapWatcher(key, ow.Events, ow.stopCh, ow.client)
	if err != nil {
		return err
	}

	ow.configmaps[key] = &cm

	return nil
}

func (ow *ObjectWatcher) AddServices(keys []types.NamespacedName) error {
	for _, key := range keys {
		if _, exists := ow.services[key]; exists {
			continue
		}

		svc, err := newServiceWatcher(key, ow.Events, ow.stopCh, ow.client)
		if err != nil {
			return err
		}

		ow.services[key] = &svc
	}

	return nil
}

func (ow *ObjectWatcher) RemoveConfigmap(key types.NamespacedName) error {
	if _, exists := ow.configmaps[key]; !exists {
		return fmt.Errorf("")
	}

	delete(ow.configmaps, key)
	return nil
}

func (ew *ObjectWatcher) Run() {
	for {
		select {
		case evt := <-ew.Events:
			// for now just show a string with event and the configmap
			klog.Infof("[K8S data change] - reason: %v", evt)
		case <-ew.stopCh:
			break
		}
	}
}
