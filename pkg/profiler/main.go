package profiler

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"

	"github.com/go-logr/logr"
)

func Register(log logr.Logger) {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/heap", pprof.Index)
	mux.HandleFunc("/debug/pprof/mutex", pprof.Index)
	mux.HandleFunc("/debug/pprof/goroutine", pprof.Index)
	mux.HandleFunc("/debug/pprof/threadcreate", pprof.Index)
	mux.HandleFunc("/debug/pprof/block", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:6060"),
		Handler: mux,
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			log.Error(err, "unable to start profile server")
			os.Exit(1)
		}
	}()
}
