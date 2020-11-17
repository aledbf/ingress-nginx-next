/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"strings"
	"time"

	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	kcache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"k8s.io/ingress-nginx-next/pkg/k8s/cache"
	"k8s.io/ingress-nginx-next/pkg/k8s/ingress"
)

// IngressReconciler reconciles a Nginx object
type IngressReconciler struct {
	Client kubernetes.Interface

	Dependencies map[types.NamespacedName]*ingress.Dependencies

	ConfigmapWatcher cache.Watcher
	EndpointsWatcher cache.Watcher
	SecretWatcher    cache.Watcher
	ServiceWatcher   cache.Watcher

	WorkQueue workqueue.RateLimitingInterface

	state kcache.Store
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress,verbs=get;list;watch;
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress/status,verbs=get;update;patch

func (r *IngressReconciler) Reconcile(object *networkingv1beta1.Ingress) error {
	key := types.NamespacedName{Name: object.Name, Namespace: object.Namespace}

	// fetch  from the cache
	ing, err := r.Client.NetworkingV1beta1().Ingresses(key.Namespace).Get(context.TODO(), key.Name, v1.GetOptions{})
	if errors.IsNotFound(err) {
		klog.InfoS("Ingress removed", "ingress", key)

		deps := r.Dependencies[key]

		r.ConfigmapWatcher.Remove(key, deps.Configmaps...)
		r.SecretWatcher.Remove(key, deps.Secrets...)
		r.ServiceWatcher.Remove(key, deps.Services...)
		r.EndpointsWatcher.Remove(key, deps.Endpoints...)

		delete(r.Dependencies, key)

		return nil
	}
	if err != nil {
		return err
	}

	deps := ingress.Parse(ing)

	r.ConfigmapWatcher.Add(key, deps.Configmaps)
	r.SecretWatcher.Add(key, deps.Secrets)
	r.ServiceWatcher.Add(key, deps.Services)
	r.EndpointsWatcher.Add(key, deps.Endpoints)

	klog.InfoS("Ingress dependencies", "ingress", deps)
	r.Dependencies[key] = deps

	return nil
}

func (r *IngressReconciler) Run(ctx context.Context) {
	informerHandlers := kcache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			r.WorkQueue.Add(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			r.WorkQueue.Add(newObj)
		},
		DeleteFunc: func(obj interface{}) {
			r.WorkQueue.Add(obj)
		},
	}

	ingressCache := cache.NewLightweightInformer(
		kcache.NewListWatchFromClient(r.Client.NetworkingV1beta1().RESTClient(), "ingresses", "", fields.Everything()),
		&networkingv1beta1.Ingress{},
		0,
		informerHandlers,
	)

	go ingressCache.Run(ctx.Done())

	klog.Info("Waiting for initial cache sync")
	kcache.WaitForCacheSync(ctx.Done(), ingressCache.HasSynced)

	klog.Info("Starting ingress process loop")
	wait.Until(
		func() {
			// Runs processNextItem in a loop, if it returns false it will
			// be restarted by wait.Until unless stopCh is sent.
			for r.processNextItem() {
			}
		},
		time.Second,
		ctx.Done(),
	)
}

func (r *IngressReconciler) processNextItem() bool {
	obj, quit := r.WorkQueue.Get()
	if quit {
		// Exit permanently
		return false
	}
	defer r.WorkQueue.Done(obj)

	ingress, ok := obj.(*networkingv1beta1.Ingress)
	if !ok {
		klog.Warning("Error decoding object, invalid type. Dropping")
		r.WorkQueue.Forget(obj)
		// short-circuit on this item, but return true to keep processing
		return true
	}

	key := types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}

	err := r.Reconcile(ingress)
	if err == nil {
		klog.InfoS("Removing from work queue", "ingress", key)
		r.WorkQueue.Forget(obj)
	} else if r.WorkQueue.NumRequeues(obj) < 50 {
		if strings.Contains(err.Error(), "the object has been modified; please apply your changes to the latest version and try again") {
			klog.V(5).InfoS("Object modified, requeue for retry", "ingress", key)
			klog.InfoS("Re-adding %s/%s to work queue", "ingress", key)
			r.WorkQueue.AddRateLimited(obj)
		} else if strings.Contains(err.Error(), "not found") {
			klog.V(5).InfoS("Object removed, dequeue", "ingress", key)
			r.WorkQueue.Forget(obj)
		} else {
			klog.ErrorS(err, "unexpected error")
			klog.InfoS("Re-adding to work queue", "ingress", key)
			r.WorkQueue.AddRateLimited(obj)
		}
	} else {
		klog.InfoS("Requeue limit reached, removing", "ingress", key)
		r.WorkQueue.Forget(obj)
		runtime.HandleError(err)
	}

	// Return true to let the loop process the next item
	return true
}
