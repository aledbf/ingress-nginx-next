package watch

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	cache "k8s.io/client-go/tools/cache"
)

type ServiceWatcher interface {
	GetService() *corev1.Service
	GetEndpoints() *corev1.Endpoints
}

type svcWatcher struct {
	svc       *corev1.Service
	endpoints *corev1.Endpoints
}

func (w *svcWatcher) GetService() *corev1.Service {
	return w.svc
}

func (w *svcWatcher) GetEndpoints() *corev1.Endpoints {
	return w.endpoints
}

func newServiceWatcher(key types.NamespacedName, eventCh chan Event, stopCh chan struct{}, client kubernetes.Interface) ServiceWatcher {
	w := &svcWatcher{}

	kubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(client, 0, key.Namespace,
		func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", key.Name).String()
		},
	)

	svcInformer := kubeInformerFactory.Core().V1().Services().Informer()

	var remove func(obj interface{})
	remove = func(obj interface{}) {
		switch obj := obj.(type) {
		case cache.DeletedFinalStateUnknown:
			remove(obj.Obj)
		default:
			w.svc = nil
			w.endpoints = nil
		}

		eventCh <- Event{
			NamespacedName: key,
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Service",
			},
			Type: RemoveEvent,
		}
	}

	svcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			w.svc = obj.(*corev1.Service)
			eventCh <- Event{
				NamespacedName: key,
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Service",
				},
				Type: AddEvent,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			if cmp.Equal(old, cur,
				cmpopts.IgnoreFields(metav1.ObjectMeta{},
					"ResourceVersion",
					"Annotations",
				),
			) {

				return
			}

			w.svc = cur.(*corev1.Service)
			eventCh <- Event{
				NamespacedName: key,
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Service",
				},
				Type: UpdateEvent,
			}
		},
		DeleteFunc: remove,
	})

	endpointsInformer := kubeInformerFactory.Core().V1().Endpoints().Informer()

	var removeEndpoint func(obj interface{})
	removeEndpoint = func(obj interface{}) {
		switch obj := obj.(type) {
		case cache.DeletedFinalStateUnknown:
			removeEndpoint(obj.Obj)
		default:
			w.endpoints = nil
		}

		eventCh <- Event{
			NamespacedName: key,
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Endpoints",
			},
			Type: RemoveEvent,
		}
	}

	endpointsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			w.endpoints = obj.(*corev1.Endpoints)
			eventCh <- Event{
				NamespacedName: key,
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Endpoints",
				},
				Type: AddEvent,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			if cmp.Equal(old, cur,
				cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")) {
				return
			}

			w.endpoints = cur.(*corev1.Endpoints)
			eventCh <- Event{
				NamespacedName: key,
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Endpoints",
				},
				Type: UpdateEvent,
			}
		},
		DeleteFunc: remove,
	})

	// start the informer in a goroutine
	go kubeInformerFactory.Start(stopCh)

	kubeInformerFactory.WaitForCacheSync(stopCh)

	return w
}
