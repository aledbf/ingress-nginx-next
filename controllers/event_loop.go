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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	"k8s.io/ingress-nginx-next/pkg/k8s/ingress"
	"k8s.io/ingress-nginx-next/pkg/k8s/watch"
	// +kubebuilder:scaffold:imports
)

// SyncController reconciles objects used in Ingress instances
type SyncController struct {
	Dependencies map[types.NamespacedName]*ingress.Dependencies

	ConfigmapWatcher watch.Watcher
	EndpointsWatcher watch.Watcher
	SecretWatcher    watch.Watcher
	ServiceWatcher   watch.Watcher

	Events chan watch.Event
}

func (r *SyncController) Run(ctx context.Context) {
	for {
		select {
		case evt := <-r.Events:
			// for now just show a string with event
			klog.V(2).InfoS("[K8S state change]", "reason", evt)

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
				// supports dynamic updates.
				// collect all upstreams? or just send this one?

				// upstreams -> service
				_, err := r.ServiceWatcher.Get(evt.NamespacedName)
				if err != nil {
					klog.ErrorS(err, "extracting service information")
					continue
				}

				klog.InfoS("Info", "service", evt.NamespacedName)

			case "Endpoints":
				// upstream servers -> endpoints
				_, err := r.EndpointsWatcher.Get(evt.NamespacedName)
				if err != nil {
					klog.ErrorS(err, "extracting endpoints information")
					continue
				}

				klog.InfoS("Info", "endpoints", evt.NamespacedName)
			case "Secret":
				_, err := r.SecretWatcher.Get(evt.NamespacedName)
				if err != nil {
					klog.ErrorS(err, "extracting endpoints information")
					continue
				}

				klog.InfoS("Info", "secret", evt.NamespacedName)
				// supports dynamic updates.
				// collect all secrets? or just send this one?
			}
		case <-ctx.Done():
			return
		}
	}
}
