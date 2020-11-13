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

	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ingressutil "k8s.io/ingress-nginx-next/pkg/ingress"
	"k8s.io/ingress-nginx-next/pkg/watch"
	// +kubebuilder:scaffold:imports
)

// IngressReconciler reconciles a Nginx object
type IngressReconciler struct {
	client.Client

	Dependencies map[string]*ingressutil.Dependencies

	ConfigmapWatcher *watch.Configmaps
	EndpointsWatcher *watch.Endpoints
	SecretWatcher    *watch.Secrets
	ServiceWatcher   *watch.Services
}

// Implement reconcile.Reconciler so the controller can reconcile objects
var _ reconcile.Reconciler = &IngressReconciler{}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress,verbs=get;list;watch;
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress/status,verbs=get;update;patch

func (r *IngressReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Sync loop", "ingress", req.NamespacedName)

	ingress := req.NamespacedName.String()

	// fetch  from the cache
	ing := &networking.Ingress{}
	err := r.Get(ctx, req.NamespacedName, ing)
	if errors.IsNotFound(err) {
		log.Info("Ingress removed", "ingress", ingress)

		r.ConfigmapWatcher.RemoveReferencedBy(ingress)
		r.EndpointsWatcher.RemoveReferencedBy(ingress)
		r.SecretWatcher.RemoveReferencedBy(ingress)
		r.ServiceWatcher.RemoveReferencedBy(ingress)

		delete(r.Dependencies, ingress)
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	deps := ingressutil.Parse(ing)

	r.ConfigmapWatcher.Add(ingress, deps.Configmaps)
	r.EndpointsWatcher.Add(ingress, deps.Services)
	r.SecretWatcher.Add(ingress, deps.Secrets)
	r.ServiceWatcher.Add(ingress, deps.Services)

	log.Info("Ingress dependencies", "ingress", deps)
	r.Dependencies[ingress] = deps

	return reconcile.Result{}, nil
}
