package watch

import (
	"fmt"
	"sync"

	kv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"k8s.io/ingress-nginx-next/pkg/util/reference"
)

type Watcher interface {
	Add(ingress types.NamespacedName, refs []types.NamespacedName)
	Remove(ingress types.NamespacedName, refs ...types.NamespacedName)

	Get(key types.NamespacedName) (runtime.Object, error)
}

type watcher struct {
	plural string

	events chan Event

	restClient rest.Interface

	references   reference.ObjectRefMap
	referencesMu *sync.RWMutex

	informerHandlers cache.ResourceEventHandlerFuncs

	cache map[types.NamespacedName]chan struct{}
	store cache.Store
}

func SingleObject(plural string, eventCh chan Event, restClient rest.Interface) Watcher {
	w := &watcher{
		plural: plural,

		events: eventCh,

		restClient: restClient,

		cache: make(map[types.NamespacedName]chan struct{}),
		store: cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),

		references:   reference.NewObjectRefMap(),
		referencesMu: &sync.RWMutex{},
	}

	w.informerHandlers = cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.store.Add(obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { w.store.Update(newObj) },
		DeleteFunc: func(obj interface{}) { w.store.Delete(obj) },
	}

	return w
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
		w.cache[key] = stopCh

		keyCache := w.newSingleCache(key)
		go keyCache.Run(stopCh)

		cache.WaitForCacheSync(stopCh, keyCache.HasSynced)
	}
}

func (w *watcher) Get(key types.NamespacedName) (runtime.Object, error) {
	if _, exists := w.cache[key]; exists {
		item, exists, err := w.store.Get(key.String())
		if err != nil {
			return nil, err
		}

		if !exists {
			return nil, fmt.Errorf("object %v does not exists", key)
		}

		return item.(runtime.Object), nil
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

		// close channel (terminates goroutine)
		close(w.cache[key])
		delete(w.cache, key)
	}
}

func (w *watcher) newSingleCache(key types.NamespacedName) cache.Controller {
	watchOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector("metadata.name", key.Name).String()
	}
	watchlist := cache.NewFilteredListWatchFromClient(w.restClient, w.plural, key.Namespace, watchOptions)
	return NewLightweightInformer(watchlist, objectFromString(w.plural), 0, w.informerHandlers)
}

func objectFromString(plural string) runtime.Object {
	switch plural {
	case "configmaps":
		return &kv1.ConfigMap{}
	case "endpoints":
		return &kv1.Endpoints{}
	case "secrets":
		return &kv1.Secret{}
	case "services":
		return &kv1.Service{}
	default:
		panic(fmt.Errorf("unexpected type"))
	}
}
