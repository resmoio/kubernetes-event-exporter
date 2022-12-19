package kube

import (
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
	eventsDiscarded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "events_discarded",
		Help: "The total number of events discarded because of being older than the maxEventAgeSeconds specified",
	})
	watchErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "watch_errors",
		Help: "The total number of errors received from the informer",
	})

	startUpTime = time.Now()
)

type EventHandler func(event *EnhancedEvent)

type EventWatcher struct {
	informer           cache.SharedInformer
	stopper            chan struct{}
	labelCache         *LabelCache
	annotationCache    *AnnotationCache
	fn                 EventHandler
	maxEventAgeSeconds time.Duration
}

func NewEventWatcher(config *rest.Config, namespace string, MaxEventAgeSeconds int64, fn EventHandler) *EventWatcher {
	clientset := kubernetes.NewForConfigOrDie(config)
	factory := informers.NewSharedInformerFactoryWithOptions(clientset, 0, informers.WithNamespace(namespace))
	informer := factory.Core().V1().Events().Informer()

	watcher := &EventWatcher{
		informer:           informer,
		stopper:            make(chan struct{}),
		labelCache:         NewLabelCache(config),
		annotationCache:    NewAnnotationCache(config),
		fn:                 fn,
		maxEventAgeSeconds: time.Second * time.Duration(MaxEventAgeSeconds),
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
	// Ignore updates
}

// Ignore events older than the maxEventAgeSeconds
func (e *EventWatcher) isEventDiscarded(event *corev1.Event) bool {
	timestamp := event.LastTimestamp.Time
	if timestamp.IsZero() {
		timestamp = event.EventTime.Time
	}
	eventAge := time.Since(timestamp)
	if eventAge > e.maxEventAgeSeconds {
		// Log discarded events if they were created after the watcher started
		// (to suppres warnings from initial synchrnization)
		if timestamp.After(startUpTime) {
			log.Warn().
				Str("event age", eventAge.String()).
				Str("event namespace", event.Namespace).
				Str("event name", event.Name).
				Msg("Event discarded as being older then maxEventAgeSeconds")
			eventsDiscarded.Inc()
		}
		return true
	}
	return false
}

func (e *EventWatcher) onEvent(event *corev1.Event) {
	if e.isEventDiscarded(event) {
		return
	}

	log.Debug().
		Str("msg", event.Message).
		Str("namespace", event.Namespace).
		Str("reason", event.Reason).
		Str("involvedObject", event.InvolvedObject.Name).
		Msg("Received event")

	eventsProcessed.Inc()

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
	// Ignore deletes
}

func (e *EventWatcher) Start() {
	go e.informer.Run(e.stopper)
}

func (e *EventWatcher) Stop() {
	e.stopper <- struct{}{}
	close(e.stopper)
}

func NewMockEventWatcher(MaxEventAgeSeconds int64) *EventWatcher {
	watcher := &EventWatcher{
		labelCache:         NewMockLabelCache(),
		annotationCache:    NewMockAnnotationCache(),
		maxEventAgeSeconds: time.Second * time.Duration(MaxEventAgeSeconds),
		fn:                 func(event *EnhancedEvent) {},
	}
	return watcher
}

func (e *EventWatcher) SetStartUpTime(time time.Time) {
	startUpTime = time
}
