package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/time/rate"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type watcher struct {
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
}

func (w *watcher) addOrUpdate(key types.NamespacedName, svc runtime.Object) {
	w.watcherMu.Lock()
	defer w.watcherMu.Unlock()

	w.watching[key.String()] = svc
}

func (w *watcher) Add(keys []types.NamespacedName) error {
	w.toWatchMu.RLock()
	defer w.toWatchMu.RUnlock()

	for _, key := range keys {
		if w.toWatch.Has(key.String()) {
			continue
		}

		w.toWatch.Insert(key.String())
	}

	// reload controller
	w.reloadQueue.Add("svc")

	return nil
}

func (w *watcher) Remove(key types.NamespacedName) error {
	w.toWatchMu.Lock()
	defer w.toWatchMu.Unlock()

	w.watcherMu.Lock()
	defer w.watcherMu.Unlock()

	if !w.toWatch.Has(key.String()) {
		return nil
	}

	delete(w.watching, key.String())

	w.toWatch.Delete(key.String())
	// reload controller
	w.reloadQueue.Add("svc")

	return nil
}

func (w *watcher) Get(key types.NamespacedName) (runtime.Object, error) {
	w.watcherMu.RLock()
	defer w.watcherMu.RUnlock()

	if obj, exists := w.watching[key.String()]; exists {
		return obj, nil
	}

	return nil, fmt.Errorf("object %v does not exists", key)
}

func NewWatcher(runObj runtime.Object, eventCh chan Event, mgr manager.Manager) (*watcher, error) {
	w := &watcher{
		runtimeObject: runObj,

		stopCh: make(chan struct{}),
		events: eventCh,

		watching:  make(map[string]runtime.Object),
		watcherMu: &sync.RWMutex{},

		mgr: mgr,

		toWatch:   sets.NewString(),
		toWatchMu: &sync.RWMutex{},

		log: ctrl.Log.WithName("watch").WithName("object"),

		reloadQueue: workqueue.NewNamedRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 1000*time.Second),
			// 10 qps, 100 bucket size. This is only for retry speed and its
			// only the overall factor (not per item).
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		), "service-reload"),
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

	time.Sleep(1 * time.Second)

	// start a new service-controller
	err := w.newServiceController()
	if err != nil {
		return false
	}

	return true
}

func (w *watcher) newServiceController() error {
	c, err := controller.NewUnmanaged("watcher-controller", w.mgr, controller.Options{
		Reconciler: reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
			obj := w.runtimeObject.DeepCopyObject()
			kind := obj.GetObjectKind()

			apiError := w.mgr.GetClient().Get(context.Background(), req.NamespacedName, obj)
			if apiError != nil {
				if apierrors.IsNotFound(apiError) {
					w.events <- Event{
						NamespacedName: req.NamespacedName,
						TypeMeta: metav1.TypeMeta{
							APIVersion: kind.GroupVersionKind().Version,
							Kind:       kind.GroupVersionKind().Kind,
						},
						Type: RemoveEvent,
					}

					err := w.Remove(req.NamespacedName)
					return reconcile.Result{}, err
				}

				return reconcile.Result{}, apiError
			}

			w.addOrUpdate(req.NamespacedName, obj)
			w.events <- Event{
				NamespacedName: req.NamespacedName,
				TypeMeta: metav1.TypeMeta{
					APIVersion: kind.GroupVersionKind().Version,
					Kind:       kind.GroupVersionKind().Kind,
				},
				Type: AddUpdateEvent,
			}

			return reconcile.Result{}, nil
		}),
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: w.runtimeObject}, &handler.EnqueueRequestForObject{}, w.predicate())
	if err != nil {
		close(w.stopCh)
		return err
	}

	go func() {
		if err := c.Start(w.stopCh); err != nil {
			w.log.Error(err, "starting controller")
		}
	}()

	return nil
}

func (w *watcher) predicate() predicate.Predicate {
	w.toWatchMu.RLock()
	defer w.toWatchMu.RUnlock()

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
	key, err := toolscache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return false
	}

	return w.toWatch.Has(key)
}
