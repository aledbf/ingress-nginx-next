package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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

const (
	errCreateCache      = "cannot create new cache"
	errCreateController = "cannot create new controller"
	errCrashCache       = "cache error"
	errCrashController  = "controller error"
	errWatch            = "cannot setup watch"
)

type watcher struct {
	name string

	events chan Event

	watcherCtx    context.Context
	watcherCancel context.CancelFunc

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

type Watcher interface {
	Add(ingress string, keys []string)
	Get(key string) (client.Object, error)
	Remove(key string)
	Start(ctx context.Context)
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

func (w *watcher) Remove(key string) {
	w.toWatchMu.Lock()
	defer w.toWatchMu.Unlock()

	w.watcherMu.Lock()
	defer w.watcherMu.Unlock()

	if !w.toWatch.Has(key) {
		return
	}

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

func NewWatcher(name string, runObj client.Object, isReferencedFn func(key string) bool, eventCh chan Event, mgr manager.Manager) (Watcher, error) {
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
	w.watcherCtx, w.watcherCancel = context.WithCancel(ctx)
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

	// cancel running context
	w.watcherCancel()

	// create a new context
	w.watcherCtx, w.watcherCancel = context.WithCancel(ctx)

	ca, err := cache.New(w.mgr.GetConfig(), cache.Options{
		Scheme: w.mgr.GetScheme(),
		Mapper: w.mgr.GetRESTMapper(),
	})

	if err != nil {
		w.log.Error(err, errCreateCache)
		return false
	}

	// start a new watcher
	c, err := w.newController(ca)
	if err != nil {
		w.log.Error(err, errCreateController)
		return false
	}

	go func() {
		if err := c.Start(w.watcherCtx); err != nil {
			w.log.Error(err, errCrashController)
		}
	}()

	go func() {
		if err = ca.Start(w.watcherCtx); err != nil {
			w.log.Error(err, errCreateCache)
		}
	}()

	if ok := ca.WaitForCacheSync(w.watcherCtx); !ok {
		w.log.Error(err, errCrashCache)
		return false
	}

	w.reloadQueue.Done(key)

	return true
}

func (w *watcher) newController(ca cache.Cache) (controller.Controller, error) {
	c, err := controller.NewUnmanaged(fmt.Sprintf("%v-controller", w.name), w.mgr, controller.Options{
		Reconciler: reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
			obj := w.runtimeObject.DeepCopyObject().(client.Object)
			watchMeta := metav1.TypeMeta{
				Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
				APIVersion: obj.GetObjectKind().GroupVersionKind().Version,
			}

			apiError := w.mgr.GetClient().Get(context.Background(), req.NamespacedName, obj)
			if apiError != nil {
				if apierrors.IsNotFound(apiError) {
					w.events <- Event{NamespacedName: req.NamespacedName.String(), Type: RemoveEvent, TypeMeta: watchMeta}
					w.Remove(req.NamespacedName.String())
					return reconcile.Result{}, nil
				}

				return reconcile.Result{}, apiError
			}

			w.addOrUpdate(req.NamespacedName.String(), obj)

			w.events <- Event{NamespacedName: req.NamespacedName.String(), Type: AddUpdateEvent, TypeMeta: watchMeta}

			return reconcile.Result{}, nil
		}),
	})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(source.NewKindWithCache(w.runtimeObject, ca), &handler.EnqueueRequestForObject{}, w.predicate()); err != nil {
		return nil, errors.Wrap(err, errCreateCache)
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

	return w.toWatch.Has(client.ObjectKeyFromObject(obj).String())
}
