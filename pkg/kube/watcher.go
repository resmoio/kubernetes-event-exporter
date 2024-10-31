package kube

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/resmoio/kubernetes-event-exporter/pkg/metrics"
)

var startUpTime = time.Now()

type EventHandler func(event *EnhancedEvent)

type EventWatcher struct {
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	informer           cache.SharedInformer
	fn                 EventHandler
	maxEventAgeSeconds time.Duration
	metricsStore       *metrics.Store
	clientset          *kubernetes.Clientset
	w                  watch.Interface
}

func NewEventWatcher(config *rest.Config, namespace string, MaxEventAgeSeconds int64, fn EventHandler) (*EventWatcher, error) {
	clientset := kubernetes.NewForConfigOrDie(config)
	ctx, cancel := context.WithCancel(context.Background())
	w, err := clientset.CoreV1().Events(namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		cancel()
		return nil, err
	}

	watcher := &EventWatcher{
		ctx:                ctx,
		cancel:             cancel,
		w:                  w,
		fn:                 fn,
		maxEventAgeSeconds: time.Second * time.Duration(MaxEventAgeSeconds),
		metricsStore:       metrics.Default,
		clientset:          clientset,
	}

	return watcher, nil
}

func (e *EventWatcher) loopHandle() {
	for {
		select {
		case evt := <-e.w.ResultChan():
			// only handles Added events
			e.metricsStore.EventsTypeReceived.WithLabelValues(string(evt.Type)).Add(1)
			if evt.Type == watch.Added {
				e.OnAdd(evt.Object)
			}

		case <-e.ctx.Done():
			return
		}
	}
}

func (e *EventWatcher) OnAdd(obj interface{}) {
	event, ok := obj.(*corev1.Event)
	if !ok {
		log.Error().Msgf("Expected Event type, but got %T", obj)
		e.metricsStore.WatchErrors.Inc()
		return
	}
	e.onEvent(event)
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
			e.metricsStore.EventsDiscarded.Inc()
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

	e.metricsStore.EventsProcessed.Inc()

	ev := &EnhancedEvent{
		Event: *event.DeepCopy(),
	}
	ev.Event.ManagedFields = nil

	ev.InvolvedObject.ObjectReference = *event.InvolvedObject.DeepCopy()
	e.fn(ev)
}

func (e *EventWatcher) Start() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.loopHandle()
	}()
}

func (e *EventWatcher) Stop() {
	e.cancel()
	e.wg.Wait()
}

func (e *EventWatcher) setStartUpTime(time time.Time) {
	startUpTime = time
}
