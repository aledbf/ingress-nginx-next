package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	ctx    context.Context
	cancel context.CancelFunc

	watching  map[string]client.Object
	watcherMu *sync.RWMutex

	toWatch   sets.String
	toWatchMu *sync.RWMutex

	mgr manager.Manager
	log logr.Logger

	reloadQueue workqueue.RateLimitingInterface

	runtimeObject client.Object

	isReferencedFn func(key string) bool
}

func (w *watcher) addOrUpdate(key string, obj client.Object) {
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
	w.reloadQueue.Add("add")
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
	w.reloadQueue.Add("remove")
}

func (w *watcher) Get(key string) (client.Object, error) {
	w.watcherMu.RLock()
	defer w.watcherMu.RUnlock()

	if obj, exists := w.watching[key]; exists {
		return obj, nil
	}

	return nil, fmt.Errorf("object %v does not exists", key)
}

func NewWatcher(name string, runObj client.Object, isReferencedFn func(key string) bool, eventCh chan Event, mgr manager.Manager) (*watcher, error) {
	w := &watcher{
		name: name,

		isReferencedFn: isReferencedFn,

		runtimeObject: runObj,

		events: eventCh,

		watching:  make(map[string]client.Object),
		watcherMu: &sync.RWMutex{},

		mgr: mgr,

		toWatch:   sets.NewString(),
		toWatchMu: &sync.RWMutex{},

		log: ctrl.Log.WithName("watcher").WithName(name),

		reloadQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), fmt.Sprintf("%v-queue", name)),
	}

	return w, nil
}

func (w *watcher) Start(ctx context.Context) {
	w.ctx, w.cancel = context.WithCancel(ctx)

	wait.UntilWithContext(ctx, func(ctx context.Context) {
		for w.processNextItem(ctx) {
		}
	}, time.Second)
}

func (w *watcher) processNextItem(ctx context.Context) bool {
	key, quit := w.reloadQueue.Get()
	if quit {
		return false
	}

	defer w.reloadQueue.Done(key)

	// stop watcher
	w.cancel()
	// create a new stop channel
	w.ctx, w.cancel = context.WithCancel(ctx)

	ca, err := cache.New(w.mgr.GetConfig(), cache.Options{
		Scheme: w.mgr.GetScheme(),
		Mapper: w.mgr.GetRESTMapper(),
	})
	if err != nil {
		return false
	}

	// start a new watcher
	c, err := w.newWatcherController(ca)
	if err != nil {
		return false
	}

	go func() {
		if err := c.Start(w.ctx); err != nil {
			w.log.Error(err, fmt.Sprintf("starting %v controller", w.name))
		}
	}()

	if err = ca.Start(w.ctx); err != nil {
		return false
	}

	if ok := ca.WaitForCacheSync(w.ctx); !ok {
		return false
	}

	return true
}

func (w *watcher) newWatcherController(ca cache.Cache) (controller.Controller, error) {
	c, err := controller.NewUnmanaged(fmt.Sprintf("%v-controller", w.name), w.mgr, controller.Options{
		Reconciler: reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
			obj := w.runtimeObject.DeepCopyObject().(client.Object)
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

func (w *watcher) shouldWatch(obj client.Object) bool {
	w.toWatchMu.RLock()
	defer w.toWatchMu.RUnlock()

	key := objectKeyToStoreKey(obj)
	return w.toWatch.Has(key)
}

func objectKeyToStoreKey(k client.Object) string {
	if k.GetNamespace() == "" {
		return "default/" + k.GetName()
	}
	return k.GetNamespace() + "/" + k.GetName()
}
