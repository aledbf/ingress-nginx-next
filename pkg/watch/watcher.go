package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type watcher struct {
	name string

	events chan Event
	stopCh chan struct{}

	watching  map[string]runtime.Object
	watcherMu *sync.RWMutex

	toWatch   sets.String
	toWatchMu *sync.RWMutex

	mgr manager.Manager
	log logr.Logger

	reloadQueue workqueue.RateLimitingInterface

	runtimeObject runtime.Object

	isReferencedFn func(key string) bool
}

func (w *watcher) addOrUpdate(key string, obj runtime.Object) {
	w.watcherMu.Lock()
	defer w.watcherMu.Unlock()

	w.watching[key] = obj
}

func (w *watcher) Add(ingress string, keys []string) {
	w.toWatchMu.Lock()
	defer w.toWatchMu.Unlock()

	for _, key := range keys {
		if w.toWatch.Has(key) {
			continue
		}

		w.toWatch.Insert(key)
	}

	// reload controller
	w.reloadQueue.Add("dummy")
}

func (w *watcher) remove(key string) {
	w.toWatchMu.Lock()
	defer w.toWatchMu.Unlock()

	w.watcherMu.Lock()
	defer w.watcherMu.Unlock()

	if !w.toWatch.Has(key) {
		return
	}

	w.log.Info("removing object from watcher", "key", key)
	delete(w.watching, key)

	if w.isReferencedFn(key) {
		return
	}

	w.toWatch.Delete(key)

	// reload controller
	w.reloadQueue.Add("dummy")
}

func (w *watcher) Get(key string) (runtime.Object, error) {
	w.watcherMu.RLock()
	defer w.watcherMu.RUnlock()

	if obj, exists := w.watching[key]; exists {
		return obj, nil
	}

	return nil, fmt.Errorf("object %v does not exists", key)
}

func NewWatcher(name string, runObj runtime.Object, isReferencedFn func(key string) bool, eventCh chan Event, mgr manager.Manager) (*watcher, error) {
	w := &watcher{
		name: name,

		isReferencedFn: isReferencedFn,

		runtimeObject: runObj,

		stopCh: make(chan struct{}),
		events: eventCh,

		watching:  make(map[string]runtime.Object),
		watcherMu: &sync.RWMutex{},

		mgr: mgr,

		toWatch:   sets.NewString(),
		toWatchMu: &sync.RWMutex{},

		log: ctrl.Log.WithName("watcher").WithName(name),

		reloadQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), fmt.Sprintf("%v-queue", name)),
	}

	return w, nil
}

func (w *watcher) Start(stopCh <-chan struct{}) {
	wait.Until(w.runWorker, time.Second, stopCh)
}

func (w *watcher) runWorker() {
	for w.processNextItem() {
	}
}

func (w *watcher) processNextItem() bool {
	key, quit := w.reloadQueue.Get()
	if quit {
		return false
	}

	defer w.reloadQueue.Done(key)

	// stop service-controler
	close(w.stopCh)
	// create a new stop channel
	w.stopCh = make(chan struct{})

	ca, err := cache.New(w.mgr.GetConfig(), cache.Options{Scheme: w.mgr.GetScheme(), Mapper: w.mgr.GetRESTMapper()})
	if err != nil {
		return false
	}

	// start a new service-controller
	c, err := w.newServiceController(ca)
	if err != nil {
		return false
	}

	go func() {
		if err := c.Start(w.stopCh); err != nil {
			w.log.Error(err, fmt.Sprintf("starting %v controller", w.name))
		}
	}()

	if err = ca.Start(w.stopCh); err != nil {
		return false
	}

	if ok := ca.WaitForCacheSync(w.stopCh); !ok {
		return false
	}

	return true
}

func (w *watcher) newServiceController(ca cache.Cache) (controller.Controller, error) {
	c, err := controller.NewUnmanaged(fmt.Sprintf("%v-controller", w.name), w.mgr, controller.Options{
		Reconciler: reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
			obj := w.runtimeObject.DeepCopyObject()
			apiError := w.mgr.GetClient().Get(context.Background(), req.NamespacedName, obj)
			meta := metav1.TypeMeta{
				Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
				APIVersion: obj.GetObjectKind().GroupVersionKind().Version,
			}

			if apiError != nil {
				if apierrors.IsNotFound(apiError) {
					w.events <- Event{NamespacedName: req.NamespacedName.String(), Type: RemoveEvent, TypeMeta: meta}
					w.remove(req.NamespacedName.String())
					return reconcile.Result{}, nil
				}

				return reconcile.Result{}, apiError
			}

			w.addOrUpdate(req.NamespacedName.String(), obj)
			w.events <- Event{NamespacedName: req.NamespacedName.String(), Type: AddUpdateEvent, TypeMeta: meta}

			return reconcile.Result{}, nil
		}),
	})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(source.NewKindWithCache(w.runtimeObject, ca), &handler.EnqueueRequestForObject{}, w.predicate()); err != nil {
		return nil, err
	}

	return c, nil
}

func (w *watcher) predicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return w.shouldWatch(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return w.shouldWatch(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return w.shouldWatch(e.Object)
		},
	}
}

func (w *watcher) shouldWatch(obj runtime.Object) bool {
	w.toWatchMu.RLock()
	defer w.toWatchMu.RUnlock()

	key, err := toolscache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return false
	}

	return w.toWatch.Has(key)
}
