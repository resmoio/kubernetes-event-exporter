package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
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
