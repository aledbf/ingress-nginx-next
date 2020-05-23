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
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type ServiceWatcher struct {
	events chan Event
	stopCh chan struct{}

	services  map[string]*corev1.Service
	serviceMu *sync.RWMutex

	toWatch   sets.String
	toWatchMu *sync.RWMutex

	mgr manager.Manager
	log logr.Logger

	reloadQueue workqueue.RateLimitingInterface
}

func (sw *ServiceWatcher) addService(key types.NamespacedName, svc *corev1.Service) {
	sw.serviceMu.Lock()
	defer sw.serviceMu.Unlock()

	sw.services[key.String()] = svc
}

func (sw *ServiceWatcher) Add(keys []types.NamespacedName) error {
	sw.toWatchMu.RLock()
	defer sw.toWatchMu.RUnlock()

	for _, key := range keys {
		if sw.toWatch.Has(key.String()) {
			continue
		}

		sw.toWatch.Insert(key.String())
	}

	// reload controller
	sw.reloadQueue.Add("svc")

	return nil
}

func (sw *ServiceWatcher) Remove(key types.NamespacedName) error {
	sw.toWatchMu.RLock()
	defer sw.toWatchMu.RUnlock()

	if !sw.toWatch.Has(key.String()) {
		return nil
	}

	sw.toWatch.Delete(key.String())
	// reload controller
	sw.reloadQueue.Add("svc")

	return nil
}

func (sw *ServiceWatcher) GetService(key types.NamespacedName) (*corev1.Service, error) {
	sw.serviceMu.RLock()
	defer sw.serviceMu.RUnlock()

	if sw, exists := sw.services[key.String()]; exists {
		return sw, nil
	}

	return nil, fmt.Errorf("service %v does not exists", key)
}

func NewServiceWatcher(eventCh chan Event, stopCh <-chan struct{}, mgr manager.Manager) (*ServiceWatcher, error) {
	sw := &ServiceWatcher{
		stopCh: make(chan struct{}),
		events: eventCh,

		services:  make(map[string]*corev1.Service),
		serviceMu: &sync.RWMutex{},

		mgr: mgr,

		toWatch:   sets.NewString(),
		toWatchMu: &sync.RWMutex{},

		log: ctrl.Log.WithName("watch").WithName("service"),

		reloadQueue: workqueue.NewNamedRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 1000*time.Second),
			// 10 qps, 100 bucket size. This is only for retry speed and its
			// only the overall factor (not per item).
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		), "reload"),
	}

	go wait.Until(sw.runWorker, time.Second, stopCh)

	return sw, nil
}

func (sw *ServiceWatcher) runWorker() {
	for sw.processNextItem() {
	}
}

func (sw *ServiceWatcher) processNextItem() bool {
	key, quit := sw.reloadQueue.Get()
	if quit {
		return false
	}

	defer sw.reloadQueue.Done(key)

	close(sw.stopCh)
	time.Sleep(1 * time.Second)

	sw.stopCh = make(chan struct{})

	err := sw.watch()
	if err != nil {
		return false
	}

	return true
}

func (sw *ServiceWatcher) watch() error {
	c, err := controller.NewUnmanaged("service-controller", sw.mgr, controller.Options{
		Reconciler: reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
			svc := &corev1.Service{}
			if err := sw.mgr.GetClient().Get(context.TODO(), req.NamespacedName, svc); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "cannot get service")
			}

			sw.addService(req.NamespacedName, svc)

			sw.events <- Event{
				NamespacedName: req.NamespacedName,
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Service",
				},
				Type: AddEvent,
			}

			return reconcile.Result{}, nil
		}),
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForObject{}, sw.predicate())
	if err != nil {
		close(sw.stopCh)
		return err
	}

	go func() {
		if err := c.Start(sw.stopCh); err != nil {
			klog.Errorf("Error starting servicewatcher controller: %v", err)
		}
	}()

	return nil
}

func (sw *ServiceWatcher) predicate() predicate.Predicate {
	sw.toWatchMu.RLock()
	defer sw.toWatchMu.RUnlock()

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			svc, ok := e.Object.(*corev1.Service)
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
			klog.Infof("%v", e.Meta)
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			klog.Infof("%v", e.Meta)
			return false
		},
	}
}
