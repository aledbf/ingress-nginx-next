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

	cache map[types.NamespacedName]*cacheEntry
}

type cacheEntry struct {
	stopCh chan struct{}

	state cache.Store
}

func SingleObject(plural string, eventCh chan Event, restClient rest.Interface) Watcher {
	return &watcher{
		plural: plural,

		events: eventCh,

		restClient: restClient,

		cache: make(map[types.NamespacedName]*cacheEntry),

		references:   reference.NewObjectRefMap(),
		referencesMu: &sync.RWMutex{},
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
		state := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)

		keyCache := w.newSingleCache(key, state)
		go keyCache.Run(stopCh)

		cache.WaitForCacheSync(stopCh, keyCache.HasSynced)
		w.cache[key] = &cacheEntry{stopCh: stopCh, state: state}
	}
}

func (w *watcher) Get(key types.NamespacedName) (runtime.Object, error) {
	if cacheEntry, exists := w.cache[key]; exists {
		item, exists, err := cacheEntry.state.Get(key.String())
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
		close(w.cache[key].stopCh)
		// delete data
		delete(w.cache, key)
	}
}

func (w *watcher) newSingleCache(key types.NamespacedName, state cache.Store) cache.Controller {
	watchOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector("metadata.name", key.Name).String()
	}
	watchlist := cache.NewFilteredListWatchFromClient(w.restClient, w.plural, key.Namespace, watchOptions)

	return cache.New(&cache.Config{
		Queue:            cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{KeyFunction: cache.MetaNamespaceKeyFunc, KnownObjects: state}),
		ListerWatcher:    watchlist,
		ObjectType:       typeFromString(w.plural),
		FullResyncPeriod: 0,
		RetryOnError:     false,

		Process: func(obj interface{}) error {
			//newest := obj.(cache.Deltas).Newest()
			return nil
		},
	})
}

func typeFromString(plural string) runtime.Object {
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
