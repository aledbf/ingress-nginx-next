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
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/ingress-nginx-next/pkg/ingress"
	"k8s.io/ingress-nginx-next/pkg/watch"
	// +kubebuilder:scaffold:imports
)

// SyncController reconciles objects related to NGINX
type SyncController struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Dependencies map[types.NamespacedName]*ingress.Dependencies

	ServiceWatcher   *watch.Services
	EndpointsWatcher *watch.Endpoints

	Events chan watch.Event
}

func (r *SyncController) Run(stopCh <-chan struct{}) {
	for {
		select {
		case evt := <-r.Events:
			// for now just show a string with event
			r.Log.V(2).Info("[K8S state change]", "reason", evt)

			switch evt.Kind {
			case "Configmap":
				fallthrough
			case "Ingress":
				// collect ingresses
				// build model
				// compare
				// update
				// reload
			case "Service":
				fallthrough
			case "Endpoints":
				svc, err := r.ServiceWatcher.Get(evt.NamespacedName)
				if err != nil {
					r.Log.Error(err, "extracting service information")
					continue
				}

				eps, err := r.EndpointsWatcher.Get(evt.NamespacedName)
				if err != nil {
					r.Log.Error(err, "extracting endpoints information")
					continue
				}

				r.Log.Info("Info", "service", svc, "endpoints", eps.UID)

				// supports dynamic updates.
				// collect all upstreams? or just send this one?

				// upstreams -> service
				// upstream servers -> endpoints

			case "Secrets":
				// supports dynamic updates.
				// collect all secrets? or just send this one?
			}
		case <-stopCh:
			return
		}
	}
}
