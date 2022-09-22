package kube

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var (
	eventsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "events_sent",
		Help: "The total number of events sent",
	})
	eventsCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "events_count",
		Help: "The current number of event sent(server side)",
	}, []string{"namespace", "name", "involved_object_namespace", "involved_object_name", "involved_object_kind", "reason", "type", "source"})
	eventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "events_total",
		Help: "The total number of events received(watcher)",
	}, []string{"namespace", "name", "involved_object_namespace", "involved_object_name", "involved_object_kind", "reason", "type", "source"})
	watchErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "watch_errors",
		Help: "The total number of errors received from the informer",
	})
)

func getMetricValues(event *corev1.Event) []string {
	return []string{
		event.ObjectMeta.Namespace,
		event.ObjectMeta.Name,
		event.InvolvedObject.Namespace,
		event.InvolvedObject.Name,
		event.InvolvedObject.Kind,
		event.Reason,
		event.Type,
		fmt.Sprintf("%s/%s", event.Source.Host, event.Source.Component),
	}
}

type EventHandler func(event *EnhancedEvent)

type EventWatcher struct {
	informer        cache.SharedInformer
	stopper         chan struct{}
	labelCache      *LabelCache
	annotationCache *AnnotationCache
	fn              EventHandler
	throttlePeriod  time.Duration
}

func NewEventWatcher(config *rest.Config, namespace string, throttlePeriod int64, fn EventHandler) *EventWatcher {
	clientset := kubernetes.NewForConfigOrDie(config)
	factory := informers.NewSharedInformerFactoryWithOptions(clientset, 0, informers.WithNamespace(namespace))
	informer := factory.Core().V1().Events().Informer()

	watcher := &EventWatcher{
		informer:        informer,
		stopper:         make(chan struct{}),
		labelCache:      NewLabelCache(config),
		annotationCache: NewAnnotationCache(config),
		fn:              fn,
		throttlePeriod:  time.Second * time.Duration(throttlePeriod),
	}

	informer.AddEventHandler(watcher)
	informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		watchErrors.Inc()
	})

	return watcher
}

func (e *EventWatcher) OnAdd(obj interface{}) {
	event := obj.(*corev1.Event)
	e.onEvent(event)
}

func (e *EventWatcher) OnUpdate(oldObj, newObj interface{}) {
	event := newObj.(*corev1.Event)
	e.onEvent(event)
}

func (e *EventWatcher) onEvent(event *corev1.Event) {
	// TODO: Re-enable this after development
	// It's probably an old event we are catching, it's not the best way but anyways
	timestamp := event.LastTimestamp.Time
	if timestamp.IsZero() {
		timestamp = event.EventTime.Time
	}
	if time.Since(timestamp) > e.throttlePeriod {
		return
	}

	log.Debug().
		Str("msg", event.Message).
		Str("clustername", event.ClusterName).
		Str("namespace", event.Namespace).
		Str("reason", event.Reason).
		Str("involvedObject", event.InvolvedObject.Name).
		Msg("Received event")

	eventsProcessed.Inc()
	eventsCount.WithLabelValues(getMetricValues(event)...).Set(float64(event.Count))
	eventsTotal.WithLabelValues(getMetricValues(event)...).Inc()

	ev := &EnhancedEvent{
		Event: *event.DeepCopy(),
	}
	ev.Event.ManagedFields = nil

	labels, err := e.labelCache.GetLabelsWithCache(&event.InvolvedObject)
	if err != nil {
		if ev.InvolvedObject.Kind != "CustomResourceDefinition" {
			log.Error().Err(err).Msg("Cannot list labels of the object")
		} else {
			log.Debug().Err(err).Msg("Cannot list labels of the object (CRD)")
		}
		// Ignoring error, but log it anyways
	} else {
		ev.InvolvedObject.Labels = labels
		ev.InvolvedObject.ObjectReference = *event.InvolvedObject.DeepCopy()
	}

	annotations, err := e.annotationCache.GetAnnotationsWithCache(&event.InvolvedObject)
	if err != nil {
		if ev.InvolvedObject.Kind != "CustomResourceDefinition" {
			log.Error().Err(err).Msg("Cannot list annotations of the object")
		} else {
			log.Debug().Err(err).Msg("Cannot list annotations of the object (CRD)")
		}
	} else {
		ev.InvolvedObject.Annotations = annotations
		ev.InvolvedObject.ObjectReference = *event.InvolvedObject.DeepCopy()
	}

	e.fn(ev)
}

func (e *EventWatcher) OnDelete(obj interface{}) {
	// try to avoid OOMKill
	event := obj.(*corev1.Event)
	eventsCount.DeleteLabelValues(getMetricValues(event)...)
	eventsTotal.DeleteLabelValues(getMetricValues(event)...)
}

func (e *EventWatcher) Start() {
	go e.informer.Run(e.stopper)
}

func (e *EventWatcher) Stop() {
	e.stopper <- struct{}{}
	close(e.stopper)
}
