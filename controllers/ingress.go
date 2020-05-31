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

	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressutil "k8s.io/ingress-nginx-next/pkg/ingress"
	"k8s.io/ingress-nginx-next/pkg/watch"
	// +kubebuilder:scaffold:imports
)

// IngressReconciler reconciles a Nginx object
type IngressReconciler struct {
	client.Client

	Log    logr.Logger
	Scheme *runtime.Scheme

	Dependencies map[string]*ingressutil.Dependencies

	ConfigmapWatcher *watch.Configmaps
	EndpointsWatcher *watch.Endpoints
	SecretWatcher    *watch.Secrets
	ServiceWatcher   *watch.Services
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress,verbs=get;list;watch;
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress/status,verbs=get;update;patch

func (r *IngressReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ingress := req.NamespacedName.String()

	ing := &networking.Ingress{}

	ctx := context.Background()
	if err := r.Get(ctx, req.NamespacedName, ing); err != nil {
		if apierrors.IsNotFound(err) {
			r.ConfigmapWatcher.RemoveReferencedBy(ingress)
			r.EndpointsWatcher.RemoveReferencedBy(ingress)
			r.SecretWatcher.RemoveReferencedBy(ingress)
			r.ServiceWatcher.RemoveReferencedBy(ingress)

			delete(r.Dependencies, ingress)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	deps := ingressutil.Parse(ing)

	r.ConfigmapWatcher.Add(ingress, deps.Configmaps)
	r.EndpointsWatcher.Add(ingress, deps.Services)
	r.SecretWatcher.Add(ingress, deps.Secrets)
	r.ServiceWatcher.Add(ingress, deps.Services)

	r.Log.V(2).Info("Ingress dependencies", "ingress", deps)
	r.Dependencies[ingress] = deps

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networking.Ingress{}).
		Complete(r)
}
