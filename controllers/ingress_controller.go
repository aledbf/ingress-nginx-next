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
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/ingress-nginx-next/pkg/ingress"
	"k8s.io/ingress-nginx-next/pkg/watch"
	// +kubebuilder:scaffold:imports
)

// IngressReconciler reconciles a Nginx object
type IngressReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	IngressState *ingress.State

	ObjectWatcher *watch.ObjectWatcher
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress,verbs=get;list;watch;
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress/status,verbs=get;update;patch

func (r *IngressReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	log := r.Log.WithValues("ingress", req.NamespacedName)
	log.Info("Syncing")

	namespacedName := req.NamespacedName

	ingress := &networking.Ingress{}
	if err := r.Get(ctx, namespacedName, ingress); err != nil {
		if apierrors.IsNotFound(err) {
			r.IngressState.Remove(namespacedName)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	state := r.IngressState.Ensure(namespacedName, ingress)
	if err := r.ObjectWatcher.AddServices(state.Services); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networking.Ingress{}).
		Complete(r)
}
