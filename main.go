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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	"k8s.io/ingress-nginx-next/controllers"
	"k8s.io/ingress-nginx-next/pkg/k8s/client"
	"k8s.io/ingress-nginx-next/pkg/k8s/ingress"
	"k8s.io/ingress-nginx-next/pkg/k8s/watch"
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
	// initialize klog/v2, can also bind to a local flagset if desired
	klogv2.InitFlags(nil)

	// In this example, we want to show you that all the lines logged
	// end up in the myfile.log. You do NOT need them in your application
	// as all these flags are set up from the command line typically
	flag.Set("alsologtostderr", "true")  // false is default, but this is informative
	flag.Set("stderrthreshold", "TRACE") // stderrthreshold defaults to ERROR, we don't want anything in stderr
	// parse klog/v2 flags
	flag.Parse()
	// make sure we flush before exiting
	defer klogv2.Flush()

	// BEGIN : hack to redirect klogv1 calls to klog v2
	// Tell klog NOT to log into STDERR. Otherwise, we risk
	// certain kinds of API errors getting logged into a directory not
	// available in a `FROM scratch` Docker container, causing us to abort
	var klogv1Flags flag.FlagSet
	klogv1.InitFlags(&klogv1Flags)
	klogv1Flags.Set("logtostderr", "true")      // By default klog v1 logs to stderr, switch that off
	klogv1Flags.Set("stderrthreshold", "TRACE") // stderrthreshold defaults to ERROR, use this if you
	// don't want anything in your stderr

	//profiler.Register(mgr)

	kubeClient, err := kubernetes.NewForConfig(client.GetConfigOrDie())
	if err != nil {
		klogv2.ErrorS(err, "unable to create an API client")
		os.Exit(1)
	}

	events := make(chan watch.Event)

	configmapWatcher := watch.SingleObject("configmaps", events, kubeClient.CoreV1().RESTClient())
	endpointsWatcher := watch.SingleObject("endpoints", events, kubeClient.CoreV1().RESTClient())
	secretWatcher := watch.SingleObject("secrets", events, kubeClient.CoreV1().RESTClient())
	serviceWatcher := watch.SingleObject("services", events, kubeClient.CoreV1().RESTClient())

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
	time.Sleep(10 * time.Second)

	klogv2.Info("done")
}
