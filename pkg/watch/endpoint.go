package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type Endpoints struct {
	events chan Event
	stopCh chan struct{}

	endpoints   map[string]*corev1.Endpoints
	endpointsMu *sync.RWMutex

	toWatch   sets.String
	toWatchMu *sync.RWMutex

	mgr manager.Manager
	log logr.Logger

	reloadQueue workqueue.RateLimitingInterface
}

func (sw *Endpoints) add(key types.NamespacedName, eps *corev1.Endpoints) {
	sw.endpointsMu.Lock()
	defer sw.endpointsMu.Unlock()

	sw.endpoints[key.String()] = eps
}

func (sw *Endpoints) Add(keys []types.NamespacedName) error {
	sw.toWatchMu.RLock()
	defer sw.toWatchMu.RUnlock()

	for _, key := range keys {
		if sw.toWatch.Has(key.String()) {
			continue
		}

		sw.toWatch.Insert(key.String())
	}

	// reload controller
	sw.reloadQueue.Add("endpoint")

	return nil
}

func (sw *Endpoints) Remove(key types.NamespacedName) error {
	sw.toWatchMu.RLock()
	defer sw.toWatchMu.RUnlock()

	if !sw.toWatch.Has(key.String()) {
		return nil
	}

	sw.toWatch.Delete(key.String())
	// reload controller
	sw.reloadQueue.Add("endpoints")

	return nil
}

func (sw *Endpoints) GetEndppoints(key types.NamespacedName) (*corev1.Endpoints, error) {
	sw.endpointsMu.RLock()
	defer sw.endpointsMu.RUnlock()

	if sw, exists := sw.endpoints[key.String()]; exists {
		return sw, nil
	}

	return nil, fmt.Errorf("endpoint %v does not exists", key)
}

func NewEndpointsWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*Endpoints, error) {
	sw := &Endpoints{
		stopCh: make(chan struct{}),
		events: eventCh,

		endpoints:   make(map[string]*corev1.Endpoints),
		endpointsMu: &sync.RWMutex{},

		mgr: mgr,

		toWatch:   sets.NewString(),
		toWatchMu: &sync.RWMutex{},

		log: ctrl.Log.WithName("watch").WithName("endpoints"),

		reloadQueue: workqueue.NewNamedRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 1000*time.Second),
			// 10 qps, 100 bucket size. This is only for retry speed and its
			// only the overall factor (not per item).
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		), "endpoints-reload"),
	}

	go wait.Until(sw.runWorker, time.Second, stopCh)

	return sw, nil
}

func (sw *Endpoints) runWorker() {
	for sw.processNextItem() {
	}
}

func (sw *Endpoints) processNextItem() bool {
	key, quit := sw.reloadQueue.Get()
	if quit {
		return false
	}

	defer sw.reloadQueue.Done(key)

	// stop endpoints-controler
	close(sw.stopCh)
	// create a new stop channel
	sw.stopCh = make(chan struct{})

	time.Sleep(1 * time.Second)

	// start a new endpoints-controller
	err := sw.newEndpointsController()
	if err != nil {
		return false
	}

	return true
}

func (sw *Endpoints) newEndpointsController() error {
	c, err := controller.NewUnmanaged("endpoints-controller", sw.mgr, controller.Options{
		Reconciler: reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
			eps := &corev1.Endpoints{}
			if err := sw.mgr.GetClient().Get(context.Background(), req.NamespacedName, eps); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "cannot get endpoints")
			}

			sw.add(req.NamespacedName, eps)

			sw.events <- Event{
				NamespacedName: req.NamespacedName,
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Endpoints",
				},
				Type: AddEvent,
			}

			return reconcile.Result{}, nil
		}),
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Endpoints{}}, &handler.EnqueueRequestForObject{}, sw.predicate())
	if err != nil {
		close(sw.stopCh)
		return err
	}

	go func() {
		if err := c.Start(sw.stopCh); err != nil {
			sw.log.Error(err, "starting controller")
		}
	}()

	return nil
}

func (sw *Endpoints) predicate() predicate.Predicate {
	sw.toWatchMu.RLock()
	defer sw.toWatchMu.RUnlock()

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			svc, ok := e.Object.(*corev1.Endpoints)
			if !ok {
				return false
			}

			key, err := toolscache.DeletionHandlingMetaNamespaceKeyFunc(svc)
			if err != nil {
				return false
			}

			return sw.toWatch.Has(key)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}
