package watch

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

type Watcher interface {
	Add(key types.NamespacedName, keys []string)
	Get(ctx context.Context, key types.NamespacedName, obj client.Object) error
	Remove(key types.NamespacedName)
	Start(ctx context.Context)
}

type watcher struct {
	name string

	events chan Event

	cache cache.Cache

	stopCtrlrFunc context.CancelFunc

	// Set of keys of of the objects to watch
	toWatch   sets.String
	toWatchMu *sync.RWMutex

	mgr manager.Manager
	log logr.Logger

	reloadQueue workqueue.RateLimitingInterface

	runtimeObject client.Object

	isReferencedFn func(key types.NamespacedName) bool
}

func (w *watcher) Add(key types.NamespacedName, keys []string) {
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

func (w *watcher) Remove(key types.NamespacedName) {
	w.toWatchMu.Lock()
	defer w.toWatchMu.Unlock()

	if !w.toWatch.Has(key.String()) {
		return
	}

	if w.isReferencedFn(key) {
		return
	}

	w.toWatch.Delete(key.String())

	// reload controller
	w.reloadQueue.Add("remove")
}

func (w *watcher) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	return w.cache.Get(ctx, key, obj)
}

func NewWatcher(name string, runObj client.Object, isReferencedFn func(key types.NamespacedName) bool, eventCh chan Event, mgr manager.Manager) (Watcher, error) {
	w := &watcher{
		name: name,

		isReferencedFn: isReferencedFn,

		runtimeObject: runObj,

		events: eventCh,

		mgr: mgr,

		toWatch:   sets.NewString(),
		toWatchMu: &sync.RWMutex{},

		log: ctrl.Log.WithName("watcher").WithName(name),

		reloadQueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(), fmt.Sprintf("%v-queue", name),
		),
	}

	return w, nil
}

func (w *watcher) Start(ctx context.Context) {
	wait.UntilWithContext(ctx, w.processNextItem, 0)
}

func (w *watcher) processNextItem(ctx context.Context) {
	key, quit := w.reloadQueue.Get()
	if quit {
		return
	}

	var ctrlCtx context.Context
	ctrlCtx, w.stopCtrlrFunc = context.WithCancel(ctx)

	controllerCache, err := cache.New(w.mgr.GetConfig(), cache.Options{
		Scheme: w.mgr.GetScheme(),
		Mapper: w.mgr.GetRESTMapper(),
	})
	if err != nil {
		w.log.Error(err, errCreateCache)
		return
	}

	ctrlr, err := w.newController(controllerCache)
	if err != nil {
		w.log.Error(err, errCreateController)
		return
	}

	go func() {
		if err := ctrlr.Start(ctrlCtx); err != nil {
			w.log.Error(err, errCrashController)
		}
	}()

	go func() {
		if err = controllerCache.Start(ctrlCtx); err != nil {
			w.log.Error(err, errCreateCache)
		}
	}()

	if ok := controllerCache.WaitForCacheSync(ctrlCtx); !ok {
		w.log.Error(err, errCrashCache)
		return
	}

	w.reloadQueue.Done(key)

	listObj := &unstructured.UnstructuredList{}
	listObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ServiceList",
	})

	err = controllerCache.List(context.Background(), listObj)
	if err != nil {
		w.log.Error(err, errCreateCache)
	}

	w.log.Info("Services", "list", listObj)

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
					w.events <- Event{NamespacedName: req.NamespacedName, Type: RemoveEvent, TypeMeta: watchMeta}
					return reconcile.Result{}, nil
				}

				return reconcile.Result{}, apiError
			}

			w.events <- Event{NamespacedName: req.NamespacedName, Type: AddUpdateEvent, TypeMeta: watchMeta}

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
