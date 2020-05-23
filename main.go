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

	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s.io/ingress-nginx-next/controllers"
	"k8s.io/ingress-nginx-next/pkg/ingress"
	"k8s.io/ingress-nginx-next/pkg/profiler"
	"k8s.io/ingress-nginx-next/pkg/watch"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = networking.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	klog.InitFlags(nil)

	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
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

	//kubeClient
	_, err = kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	profiler.Register(ctrl.Log)

	ingressDeps := ingress.NewDependenciesHolder()

	events := make(chan watch.Event)
	stopCh := ctrl.SetupSignalHandler()

	serviceWatcher, err := watch.NewServiceWatcher(events, stopCh, mgr)
	if err != nil {
		setupLog.Error(err, "unable to start service watcher")
		os.Exit(1)
	}

	endpointsWatcher, err := watch.NewEndpointsWatcher(events, stopCh, mgr)
	if err != nil {
		setupLog.Error(err, "unable to start endpoints watcher")
		os.Exit(1)
	}

	if err = (&controllers.IngressReconciler{
		Client:           mgr.GetClient(),
		Log:              ctrl.Log.WithName("controllers").WithName("ingress"),
		Scheme:           mgr.GetScheme(),
		Dependencies:     ingressDeps,
		ServiceWatcher:   serviceWatcher,
		EndpointsWatcher: endpointsWatcher,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ingress")
		os.Exit(1)
	}

	go func() {
		(&controllers.SyncController{
			Client:           mgr.GetClient(),
			Log:              ctrl.Log.WithName("controllers").WithName("sync"),
			Scheme:           mgr.GetScheme(),
			Dependencies:     ingressDeps,
			ServiceWatcher:   serviceWatcher,
			EndpointsWatcher: endpointsWatcher,

			Events: events,
		}).Run(stopCh)
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(stopCh); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder
}
