package main

import (
	"context"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opsgenie/kubernetes-event-exporter/pkg/exporter"
	"github.com/opsgenie/kubernetes-event-exporter/pkg/kube"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

var (
	conf          = flag.String("conf", "config.yaml", "The config path file")
	addr          = flag.String("metrics-address", ":2112", "The address to listen on for HTTP requests.")
	strictCaching = flag.Bool("strict-caching", false, "Include the resourceVersion in the cache key to be more strict on cache accuracy")
)

func main() {
	flag.Parse()
	b, err := ioutil.ReadFile(*conf)

	if err != nil {
		log.Fatal().Err(err).Msg("cannot read config file")
	}

	b = []byte(os.ExpandEnv(string(b)))

	var cfg exporter.Config
	err = yaml.Unmarshal(b, &cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot parse config to YAML")
	}

	log.Logger = log.With().Caller().Logger().Level(zerolog.DebugLevel)

	if cfg.LogLevel != "" {
		level, err := zerolog.ParseLevel(cfg.LogLevel)
		if err != nil {
			log.Fatal().Err(err).Str("level", cfg.LogLevel).Msg("Invalid log level")
		}
		log.Logger = log.Logger.Level(level)
	}

	if cfg.LogFormat == "json" {
		// Defaults to JSON already nothing to do
	} else if cfg.LogFormat == "" || cfg.LogFormat == "pretty" {
		log.Logger = log.Logger.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			NoColor:    false,
			TimeFormat: time.RFC3339,
		})
	} else {
		log.Fatal().Str("log_format", cfg.LogFormat).Msg("Unknown log format")
	}

	if cfg.ThrottlePeriod == 0 {
		cfg.ThrottlePeriod = 5
	}

	kubeconfig, err := kube.GetKubernetesConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("cannot get kubeconfig")
	}
	kubeconfig.QPS = cfg.KubeQPS
	kubeconfig.Burst = cfg.KubeBurst

	engine := exporter.NewEngine(&cfg, &exporter.ChannelBasedReceiverRegistry{})
	onEvent := engine.OnEvent
	if len(cfg.ClusterName) != 0 {
		onEvent = func(event *kube.EnhancedEvent) {
			// note that per code this value is not set anywhere on the kubernetes side
			// https://github.com/kubernetes/apimachinery/blob/v0.22.4/pkg/apis/meta/v1/types.go#L276
			event.ClusterName = cfg.ClusterName
			engine.OnEvent(event)
		}
	}

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
	go http.ListenAndServe(*addr, nil)

	cacheKeyGetter := kube.DefaultCacheKeyGetter
	if *strictCaching {
		log.Info().Msg("Using strict cache keys")
		cacheKeyGetter = kube.EnhancedEventCacheKeyGetter
	}

	w := kube.NewEventWatcherWithKey(kubeconfig, cfg.Namespace, cfg.ThrottlePeriod, onEvent, cacheKeyGetter)

	if cfg.LeaderElection.Enabled {

		// when we get the signal shutdown the context
		// and get out of Run
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		l, err := kube.NewLeaderElector(cfg.LeaderElection.LeaderElectionID, kubeconfig,
			func(_ context.Context) {
				log.Info().Msg("leader election won")
				w.Start()
			},
			func() {
				log.Error().Msg("leader election lost")
			},
			func(identity string) {
				if identity == kube.GetLeaderElectionID(cfg.LeaderElection.LeaderElectionID) {
					// its me
					// its my own lock
					// do nothing
					log.Info().Msgf("I was elected: %s", identity)
					return
				}
				log.Info().Msgf("new leader elected: %s", identity)
			},
		)
		if err != nil {
			log.Fatal().Err(err).Msg("create leaderelector failed")
		}

		// stop here and run
		// this allows us to keep watching until the next guy gets the lease
		// Run starts the leader election loop. Run will not return
		// before leader election loop is stopped by ctx or it has
		// stopped holding the leader lease
		// https://github.com/kubernetes/client-go/blob/master/tools/leaderelection/leaderelection.go#L197
		l.Run(ctx)
		// we either got a signal or we lost the lease
		// we need to wait LeaseDuration to stop watching
		// we can't stop pulling events until somebody else takes over
		// so we sleep for LeaseDuration
		log.Info().Msgf("we got the signal. waiting leaseDuration seconds to stop: %s", kube.GetLeaseDuration())
		time.Sleep(kube.GetLeaseDuration())
	} else {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		w.Start()
		select {
		case sig := <-c:
			log.Info().Str("signal", sig.String()).Msg("Received signal to exit")
		}
	}

	gracefulExit := func() {
		w.Stop()
		engine.Stop()
		log.Info().Msg("Exiting")
	}

	log.Info().Msg("Received signal to exit")
	gracefulExit()
}
