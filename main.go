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

package main

import (
	"flag"
	"os"
	"time"

	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"k8s.io/ingress-nginx-next/controllers"
	"k8s.io/ingress-nginx-next/pkg/k8s/cache"
	"k8s.io/ingress-nginx-next/pkg/k8s/client"
	"k8s.io/ingress-nginx-next/pkg/k8s/ingress"
	"k8s.io/ingress-nginx-next/pkg/util/signals"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = networking.AddToScheme(scheme)
}

func main() {
	klog.InitFlags(nil)
	defer klog.Flush()

	flag.Set("alsologtostderr", "true")
	flag.Parse()

	//profiler.Register(mgr)

	cfg, err := client.GetConfig()
	if err != nil {
		klog.ErrorS(err, "unable to create an API configuration")
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.ErrorS(err, "unable to create an API client")
		os.Exit(1)
	}

	events := make(chan cache.Event)

	configmapWatcher := cache.SingleObject(schema.FromAPIVersionAndKind("v1", "Configmap"), events, kubeClient.CoreV1().RESTClient())
	endpointsWatcher := cache.SingleObject(schema.FromAPIVersionAndKind("v1", "Endpoints"), events, kubeClient.CoreV1().RESTClient())
	secretWatcher := cache.SingleObject(schema.FromAPIVersionAndKind("v1", "Secret"), events, kubeClient.CoreV1().RESTClient())
	serviceWatcher := cache.SingleObject(schema.FromAPIVersionAndKind("v1", "Service"), events, kubeClient.CoreV1().RESTClient())

	ingressDependencies := make(map[types.NamespacedName]*ingress.Dependencies)

	ctx := signals.SetupSignalHandler()

	go func() {
		(&controllers.SyncController{
			Dependencies: ingressDependencies,

			ConfigmapWatcher: configmapWatcher,
			EndpointsWatcher: endpointsWatcher,
			SecretWatcher:    secretWatcher,
			ServiceWatcher:   serviceWatcher,

			Events: events,
		}).Run(ctx)
	}()

	go func() {
		(&controllers.IngressReconciler{
			Client: kubeClient,

			Dependencies: ingressDependencies,

			ConfigmapWatcher: configmapWatcher,
			EndpointsWatcher: endpointsWatcher,
			SecretWatcher:    secretWatcher,
			ServiceWatcher:   serviceWatcher,

			WorkQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ingress"),
		}).Run(ctx)
	}()

	<-ctx.Done()
	// additional shutdown tasks
	time.Sleep(5 * time.Second)
	klog.Info("done")
}
