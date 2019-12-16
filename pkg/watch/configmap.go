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

type configmapWatcher interface {
	GetConfigMap() (*corev1.ConfigMap, error)
}

func newConfigmapWatcher(key types.NamespacedName, eventCh chan Event, stopCh chan struct{}, client kubernetes.Interface) (configmapWatcher, error) {
	w, err := watchConfigmap(key, eventCh, stopCh, client)
	if err != nil {
		return nil, err
	}

	return w, nil
}

type watcher struct {
	object *corev1.ConfigMap
}

func (w *watcher) GetConfigMap() (*corev1.ConfigMap, error) {
	return w.object, nil
}

func watchConfigmap(key types.NamespacedName, eventCh chan Event, stopCh chan struct{}, client kubernetes.Interface) (*watcher, error) {
	kubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(client, 0, key.Namespace,
		func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", key.Name).String()
		},
	)

	w := &watcher{}

	informer := kubeInformerFactory.Core().V1().ConfigMaps().Informer()

	var remove func(obj interface{})
	remove = func(obj interface{}) {
		switch obj := obj.(type) {
		case cache.DeletedFinalStateUnknown:
			remove(obj.Obj)
		default:
			w.object = nil
		}

		eventCh <- Event{
			NamespacedName: key,
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Configmap",
			},
			Type: RemoveEvent,
		}
	}

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			w.object = obj.(*corev1.ConfigMap)
			eventCh <- Event{
				NamespacedName: key,
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Configmap",
				},
				Type: AddEvent,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			if cmp.Equal(old, cur,
				cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")) {
				return
			}

			w.object = cur.(*corev1.ConfigMap)
			eventCh <- Event{
				NamespacedName: key,
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Configmap",
				},
				Type: UpdateEvent,
			}
		},
		DeleteFunc: remove,
	})

	// start the informer in a goroutine
	go kubeInformerFactory.Start(stopCh)

	return w, nil
}
