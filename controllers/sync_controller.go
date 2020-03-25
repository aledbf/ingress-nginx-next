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
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
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

	IngressState *ingress.State

	ObjectWatcher *watch.ObjectWatcher
}

func (r *SyncController) Run(stopCh <-chan struct{}) {
	for {
		select {
		case evt := <-r.ObjectWatcher.Events:
			// for now just show a string with event
			klog.Infof("[K8S state change] - reason: %v", evt)
			switch evt.Kind {
			case "Ingress":
			case "Service":
				fallthrough
			case "Endpoints":
				svc, err := r.ObjectWatcher.GetService(evt.NamespacedName)
				if err != nil {
					klog.Errorf("%v", err)
				}

				klog.Infof("%v, %v", svc.Definition(), svc.Endpoints())
			case "Configmap":
			case "Secrets":
			}
		case <-stopCh:
			return
		}
	}
}
