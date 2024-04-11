package sinks

import (
	"context"
	"strings"

	"k8s.io/utils/strings/slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/rs/zerolog/log"
)

func newGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	v := prometheus.NewGaugeVec(opts, labelNames)
	prometheus.MustRegister(v)
	return v
}

type PrometheusConfig struct {
	EventsMetricsNamePrefix string              `yaml:"eventsMetricsNamePrefix"`
	ReasonFilter            map[string][]string `yaml:"reasonFilter"`
}

type PrometheusGaugeVec interface {
	With(labels prometheus.Labels) prometheus.Gauge
	Delete(labels prometheus.Labels) bool
}

type PrometheusSink struct {
	cfg           *PrometheusConfig
	kinds         []string
	metricsByKind map[string]PrometheusGaugeVec
}

func NewPrometheusSink(config *PrometheusConfig) (Sink, error) {
	if config.EventsMetricsNamePrefix == "" {
		config.EventsMetricsNamePrefix = "event_exporter_"
	}

	metricsByKind := map[string]PrometheusGaugeVec{}

	log.Info().Msgf("Initializing new Prometheus sink...")
	kinds := []string{}
	for kind := range config.ReasonFilter {
		kinds = append(kinds, kind)
		metricName := config.EventsMetricsNamePrefix + strings.ToLower(kind) + "_event_count"
		metricLabels := []string{strings.ToLower(kind), "namespace", "reason"}
		metricsByKind[kind] = newGaugeVec(
			prometheus.GaugeOpts{
				Name: metricName,
				Help: "Event counts for " + kind + " resources.",
			}, metricLabels)

		log.Info().Msgf("Created metric: %s, will emit events: %v with additional labels: %v", kind, config.ReasonFilter[kind], metricLabels)
	}

	return &PrometheusSink{
		cfg:           config,
		kinds:         kinds,
		metricsByKind: metricsByKind,
	}, nil
}

func (o *PrometheusSink) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	kind := ev.InvolvedObject.Kind
	if slices.Contains(o.kinds, kind) {
		for _, reason := range o.cfg.ReasonFilter[kind] {
			if ev.Reason == reason {
				SetEventCount(o.metricsByKind[kind], ev.InvolvedObject, reason, ev.Count)
			} else {
				DeleteEventCount(o.metricsByKind[kind], ev.InvolvedObject, reason)
			}
		}
	}

	return nil
}

func (o *PrometheusSink) Close() {
	// No-op
}

func getMetricLabels(obj kube.EnhancedObjectReference, reason string) prometheus.Labels {
	prometheusLabels := prometheus.Labels{
		strings.ToLower(obj.Kind): obj.Name,
		"namespace":               obj.Namespace,
		"reason":                  reason,
	}

	return prometheusLabels
}

func SetEventCount(metric PrometheusGaugeVec, obj kube.EnhancedObjectReference, reason string, count int32) {
	labels := getMetricLabels(obj, reason)
	log.Info().Msgf("Setting event count metric with labels: %v", labels)
	metric.With(labels).Set(float64(count))
}

func DeleteEventCount(metric PrometheusGaugeVec, obj kube.EnhancedObjectReference, reason string) {
	labels := getMetricLabels(obj, reason)
	log.Info().Msgf("Deleting event count metric with labels: %v", labels)
	metric.Delete(labels)
}
