package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/resmoio/kubernetes-event-exporter/pkg/version"
	"github.com/rs/zerolog/log"
)

const DefaultRuleName = "default"

type Store struct {
	EventsProcessed      prometheus.Counter
	EventsDiscarded      prometheus.Counter
	WatchErrors          prometheus.Counter
	SendErrors           prometheus.Counter
	BuildInfo            prometheus.GaugeFunc
	KubeApiReadCacheHits prometheus.Counter
	KubeApiReadRequests  prometheus.Counter
	EventsMatched		 map[string]prometheus.Counter
}

// promLogger implements promhttp.Logger
type promLogger struct{}

func (pl promLogger) Println(v ...interface{}) {
	log.Logger.Error().Msg(fmt.Sprint(v...))
}

// promLogger implements the Logger interface
func (pl promLogger) Log(v ...interface{}) error {
	log.Logger.Info().Msg(fmt.Sprint(v...))
	return nil
}

func Init(addr string, tlsConf string) {
	// Setup the prometheus metrics machinery
	// Add Go module build info.
	prometheus.MustRegister(collectors.NewBuildInfoCollector())

	promLogger := promLogger{}
	metricsPath := "/metrics"

	// Expose the registered metrics via HTTP.
	http.Handle(metricsPath, promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))

	landingConfig := web.LandingConfig{
		Name:        "kubernetes-event-exporter",
		Description: "Export Kubernetes Events to multiple destinations with routing and filtering",
		Links: []web.LandingLinks{
			{
				Address: metricsPath,
				Text:    "Metrics",
			},
		},
	}
	landingPage, _ := web.NewLandingPage(landingConfig)
	http.Handle("/", landingPage)

	http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	http.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	metricsServer := http.Server{
		ReadHeaderTimeout: 5 * time.Second}

	metricsFlags := web.FlagConfig{
		WebListenAddresses: &[]string{addr},
		WebSystemdSocket:   new(bool),
		WebConfigFile:      &tlsConf,
	}

	// start up the http listener to expose the metrics
	go web.ListenAndServe(&metricsServer, &metricsFlags, promLogger)
}

func NewMetricsStore(name_prefix string, matchRouteNames []string) *Store {
	eventsMatched := make(map[string]prometheus.Counter)
	hasDefault := false
	for i := range matchRouteNames {
		if matchRouteNames[i] == DefaultRuleName {
			hasDefault = true
		}
		eventsMatched[matchRouteNames[i]] = promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "events_matched",
			Help: "The total number of events matched by the route",
			ConstLabels: prometheus.Labels{
				"route": matchRouteNames[i],
			},
		})
	}
	if !hasDefault {
		eventsMatched[DefaultRuleName] = promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "events_matched",
			Help: "The total number of events matched by the route",
			ConstLabels: prometheus.Labels{
				"route": DefaultRuleName,
			},
		})
	}
	return &Store{
		BuildInfo: promauto.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: name_prefix + "build_info",
				Help: "A metric with a constant '1' value labeled by version, revision, branch, and goversion from which Kubernetes Event Exporter was built.",
				ConstLabels: prometheus.Labels{
					"version":   version.Version,
					"revision":  version.Revision(),
					"goversion": version.GoVersion,
					"goos":      version.GoOS,
					"goarch":    version.GoArch,
				},
			},
			func() float64 { return 1 },
		),
		EventsProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "events_sent",
			Help: "The total number of events processed",
		}),
		EventsDiscarded: promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "events_discarded",
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
		KubeApiReadCacheHits: promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "kube_api_read_cache_hits",
			Help: "The total number of read requests served from cache when looking up object metadata",
		}),
		KubeApiReadRequests: promauto.NewCounter(prometheus.CounterOpts{
			Name: name_prefix + "kube_api_read_cache_misses",
			Help: "The total number of read requests served from kube-apiserver when looking up object metadata",
		}),
		EventsMatched: eventsMatched,
	}
}

func DestroyMetricsStore(store *Store) {
	prometheus.Unregister(store.EventsProcessed)
	prometheus.Unregister(store.EventsDiscarded)
	prometheus.Unregister(store.WatchErrors)
	prometheus.Unregister(store.SendErrors)
	prometheus.Unregister(store.BuildInfo)
	prometheus.Unregister(store.KubeApiReadCacheHits)
	prometheus.Unregister(store.KubeApiReadRequests)
	for _, counter := range store.EventsMatched {
		prometheus.Unregister(counter)
	}
	store = nil
}
