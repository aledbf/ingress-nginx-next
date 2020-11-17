package cache

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/markbates/inflect"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"k8s.io/ingress-nginx-next/pkg/util/reference"
)

var (
	scheme = runtime.NewScheme()
)

type Watcher interface {
	Add(ingress types.NamespacedName, refs []types.NamespacedName)
	Remove(ingress types.NamespacedName, refs ...types.NamespacedName)
}

type watcher struct {
	gvk schema.GroupVersionKind

	events chan Event

	restClient rest.Interface

	references   reference.ObjectRefMap
	referencesMu *sync.RWMutex

	informerHandlers cache.ResourceEventHandlerFuncs

	cache map[types.NamespacedName]chan struct{}
	store cache.Store
}

func SingleObject(gvk schema.GroupVersionKind, eventCh chan Event, restClient rest.Interface) Watcher {
	w := &watcher{
		gvk: gvk,

		events: eventCh,

		restClient: restClient,

		cache: make(map[types.NamespacedName]chan struct{}),
		store: cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),

		references:   reference.NewObjectRefMap(),
		referencesMu: &sync.RWMutex{},
	}

	w.informerHandlers = cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			w.store.Add(obj)
			sendEvent(AddOrUpdateEvent, w.events, obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			w.store.Add(newObj)
			sendEvent(AddOrUpdateEvent, w.events, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			w.store.Delete(obj)
			sendEvent(RemoveEvent, w.events, obj)
		},
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
		// delete data
		delete(w.cache, key)
	}
}

func (w *watcher) newSingleCache(key types.NamespacedName) cache.Controller {
	watchOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector("metadata.name", key.Name).String()
	}

	plural := strings.ToLower(inflect.Pluralize(w.gvk.Kind))
	obj, _ := scheme.New(w.gvk)

	watchlist := cache.NewFilteredListWatchFromClient(w.restClient, plural, key.Namespace, watchOptions)
	return NewLightweightInformer(watchlist, obj, 0, w.informerHandlers)
}

func sendEvent(evtType EventType, eventCh chan Event, obj interface{}) {
	m, err := getObjectMeta(obj)
	if err != nil {
		klog.Error("Invalid Object: %T", obj)
		return
	}

	eventCh <- Event{
		Type: AddOrUpdateEvent,
		NamespacedName: types.NamespacedName{
			Name:      m.GetName(),
			Namespace: m.GetNamespace(),
		},
	}
}

var (
	errNotAPIObject = fmt.Errorf("Object is not an API Object")
)

// getObjectMeta returns the ObjectMeta if its an API object, error otherwise
func getObjectMeta(obj interface{}) (metav1.Object, error) {
	if obj == nil {
		return nil, errNotAPIObject
	}

	switch t := obj.(type) {
	case metav1.ObjectMetaAccessor:
		if reflect.ValueOf(t) == reflect.Zero(reflect.TypeOf(t)) {
			return nil, errNotAPIObject
		}
		if meta := t.GetObjectMeta(); meta != nil {
			return meta, nil
		}
	}

	return nil, errNotAPIObject
}
