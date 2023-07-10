package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/version"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)


type Store struct {
	EventsProcessed prometheus.Counter
	EventsDiscarded prometheus.Counter
	WatchErrors     prometheus.Counter
	SendErrors	    prometheus.Counter
}

func Init(addr string) {
	// Setup the prometheus metrics machinery
	// Add Go module build info.
	prometheus.MustRegister(collectors.NewBuildInfoCollector())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Kubernetes Events Exporter</title></head>
             <body>
             <h1>Kubernetes Events Exporter</h1>
             <p><a href='metrics'>Metrics</a></p>
			 <h2>Build</h2>
             <pre>` + version.Info() + ` ` + version.BuildContext() + `</pre>
             </body>
             </html>`))
	})
	http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	http.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))

	// start up the http listener to expose the metrics
	go http.ListenAndServe(addr, nil)
}

func NewMetricsStore(name_prefix string) *Store {
	return &Store{
		EventsProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "events_sent",
			Help: "The total number of events processed",
		}),
		EventsDiscarded: promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix  + "events_discarded",
			Help: "The total number of events discarded because of being older than the maxEventAgeSeconds specified",
		}),
		WatchErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "watch_errors",
			Help: "The total number of errors received from the informer",
		}),
		SendErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "send_event_errors",
			Help: "The total number of send event errors",
		}),
	}
}

func DestroyMetricsStore(store *Store) {
	prometheus.Unregister(store.EventsProcessed)
	prometheus.Unregister(store.EventsDiscarded)
	prometheus.Unregister(store.WatchErrors)
	prometheus.Unregister(store.SendErrors)
	store = nil
}
