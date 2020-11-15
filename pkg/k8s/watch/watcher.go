package watch

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	apiwatch "k8s.io/client-go/tools/watch"
	"k8s.io/ingress-nginx-next/pkg/reference"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Watcher interface {
	Add(ingress types.NamespacedName, refs []types.NamespacedName)
	Remove(ingress types.NamespacedName, refs ...types.NamespacedName)

	Get(key types.NamespacedName) (runtime.Object, error)
}

type watcher struct {
	groupKind schema.GroupVersionKind

	events chan Event

	mgr manager.Manager

	log logr.Logger

	references   reference.ObjectRefMap
	referencesMu *sync.RWMutex

	cache map[types.NamespacedName]*cacheEntry
}

type cacheEntry struct {
	stopCh chan struct{}

	obj runtime.Object
}

func SingleObject(groupKind schema.GroupVersionKind, eventCh chan Event, mgr manager.Manager) Watcher {
	return &watcher{
		groupKind: groupKind,

		events: eventCh,

		mgr: mgr,

		log: ctrl.Log.WithName("watcher").WithName(groupKind.Kind),

		references:   reference.NewObjectRefMap(),
		referencesMu: &sync.RWMutex{},

		cache: make(map[types.NamespacedName]*cacheEntry),
	}
}

func (w *watcher) Add(fromIngress types.NamespacedName, keys []types.NamespacedName) {
	w.referencesMu.Lock()
	defer w.referencesMu.Unlock()

	for _, key := range keys {
		w.references.Insert(fromIngress, key)
		if _, ok := w.cache[key]; ok {
			continue
		}

		stopCh := make(chan struct{})
		w.cache[key] = &cacheEntry{stopCh: stopCh}

		initialRevision := "1"

		if w.groupKind.Kind == "Endpoints" {
			// RetryWatcher is expected to deliver all events but cannot recover from "too old resource version"
			// For this reason we need to get the last version for the namespace and not the object
			// (https://github.com/kubernetes/kubernetes/pull/93777#discussion_r467932080).
			cfg, _ := config.GetConfig()
			kubeclientset := kubernetes.NewForConfigOrDie(cfg)
			initResource, err := kubeclientset.CoreV1().Endpoints(key.Namespace).List(context.Background(), metav1.ListOptions{})
			if err != nil {
				w.log.Error(err, "obtaining endpoint")
			} else {
				initialRevision = initResource.GetResourceVersion()
			}
		}

		go w.newWatch(key, w.events, stopCh, initialRevision)
	}
}

func (w *watcher) Get(key types.NamespacedName) (runtime.Object, error) {
	if cacheEntry, exists := w.cache[key]; exists {
		return cacheEntry.obj, nil
	}

	return nil, fmt.Errorf("object %v does not exists", key)
}

func (w *watcher) Remove(fromIngress types.NamespacedName, keys ...types.NamespacedName) {
	w.referencesMu.Lock()
	defer w.referencesMu.Unlock()

	if !w.references.HasConsumer(fromIngress) {
		return
	}
	w.references.Delete(fromIngress)

	for _, key := range keys {
		if len(w.references.Reference(key)) > 0 {
			// still referenced
			continue
		}

		w.log.Info("deleting", "key", key)

		// close channel (terminates goroutine)
		close(w.cache[key].stopCh)
		// delete data
		delete(w.cache, key)
	}
}

func (w *watcher) newWatch(key types.NamespacedName, eventCh chan Event, stopCh <-chan struct{}, initialRevision string) {
	listWatch, err := createStructuredListWatch(key, w.groupKind, w.mgr.GetRESTMapper())
	if err != nil {
		w.log.Error(err, "creating new watcher", "key", key)
		return
	}

	retryWatcher, err := apiwatch.NewRetryWatcher(initialRevision, listWatch)
	if err != nil {
		w.log.Error(err, "creating new watcher", "key", key)
		return
	}

	defer func() {
		w.log.V(2).Info("Stopping watcher", "key", key)
		retryWatcher.Stop()
	}()

	for {
		select {
		case event := <-retryWatcher.ResultChan():
			switch event.Type {
			case watch.Added, watch.Modified:
				eventCh <- Event{NamespacedName: key, Type: AddOrUpdateEvent, TypeMeta: convertToTypeMeta(event.Object)}
				w.cache[key].obj = event.Object
			case watch.Deleted:
				eventCh <- Event{NamespacedName: key, Type: RemoveEvent, TypeMeta: convertToTypeMeta(event.Object)}
				w.cache[key].obj = nil
			case watch.Error:
				errObject := apierrors.FromObject(event.Object)
				statusErr, ok := errObject.(*apierrors.StatusError)
				if !ok {
					w.log.Error(errObject, "watching object")
					continue
				}

				status := statusErr.ErrStatus
				w.log.Error(nil, "watching object", "status", status.Reason, "message", status.Message)
			default:
			}
		case <-stopCh:
			w.log.V(2).Info("Closing object watcher", "key", key)
			return
		}
	}
}

func convertToTypeMeta(obj runtime.Object) metav1.TypeMeta {
	gvks, _, _ := scheme.ObjectKinds(obj)
	kind := gvks[0]

	return metav1.TypeMeta{
		Kind:       kind.Kind,
		APIVersion: kind.GroupVersion().String(),
	}
}
