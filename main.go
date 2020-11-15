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

	kcorev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s.io/ingress-nginx-next/controllers"
	"k8s.io/ingress-nginx-next/pkg/k8s/ingress"
	"k8s.io/ingress-nginx-next/pkg/k8s/watch"
	"k8s.io/ingress-nginx-next/pkg/util/profiler"
	"k8s.io/ingress-nginx-next/pkg/util/signals"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = networking.AddToScheme(scheme)
}

func main() {
	klog.InitFlags(nil)

	var (
		metricsAddr          string
		enableLeaderElection bool
		development          bool
	)

	flag.BoolVar(&development, "development-log", true, "Configure logs in development format.")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = development
	}))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	profiler.Register(mgr)

	//kubeClient
	_, err = kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "unable to create an API client")
		os.Exit(1)
	}

	events := make(chan watch.Event)

	configmapWatcher := watch.SingleObject(kcorev1.SchemeGroupVersion.WithKind("Configmap"), events, mgr)
	endpointsWatcher := watch.SingleObject(kcorev1.SchemeGroupVersion.WithKind("Endpoints"), events, mgr)
	secretWatcher := watch.SingleObject(kcorev1.SchemeGroupVersion.WithKind("Secret"), events, mgr)
	serviceWatcher := watch.SingleObject(kcorev1.SchemeGroupVersion.WithKind("Service"), events, mgr)

	ingressDependencies := make(map[types.NamespacedName]*ingress.Dependencies)

	ctx := signals.SetupSignalHandler()
	go func() {
		(&controllers.SyncController{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("sync"),
			Scheme: mgr.GetScheme(),

			Dependencies: ingressDependencies,

			ConfigmapWatcher: configmapWatcher,
			EndpointsWatcher: endpointsWatcher,
			SecretWatcher:    secretWatcher,
			ServiceWatcher:   serviceWatcher,

			Events: events,
		}).Run(ctx)
	}()

	err = ctrl.NewControllerManagedBy(mgr).
		For(&networking.Ingress{}).
		Complete(&controllers.IngressReconciler{
			Client: mgr.GetClient(),

			Dependencies: ingressDependencies,

			ConfigmapWatcher: configmapWatcher,
			EndpointsWatcher: endpointsWatcher,
			SecretWatcher:    secretWatcher,
			ServiceWatcher:   serviceWatcher,
		})
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ingress")
		os.Exit(1)
	}

	setupLog.Info("starting ingress controller")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running ingress controller")
		os.Exit(1)
	}

	<-ctx.Done()
	// additional shutdown tasks
	time.Sleep(10 * time.Second)

	setupLog.Info("done")
}
