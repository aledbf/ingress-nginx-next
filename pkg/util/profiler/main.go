package profiler

import (
	"net/http"
	"net/http/pprof"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func Register(mgr manager.Manager) {
	mgr.AddMetricsExtraHandler("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mgr.AddMetricsExtraHandler("/debug/pprof/heap", http.HandlerFunc(pprof.Index))
	mgr.AddMetricsExtraHandler("/debug/pprof/mutex", http.HandlerFunc(pprof.Index))
	mgr.AddMetricsExtraHandler("/debug/pprof/goroutine", http.HandlerFunc(pprof.Index))
	mgr.AddMetricsExtraHandler("/debug/pprof/threadcreate", http.HandlerFunc(pprof.Index))
	mgr.AddMetricsExtraHandler("/debug/pprof/block", http.HandlerFunc(pprof.Index))
	mgr.AddMetricsExtraHandler("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mgr.AddMetricsExtraHandler("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mgr.AddMetricsExtraHandler("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mgr.AddMetricsExtraHandler("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
}
