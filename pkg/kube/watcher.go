package kube

import (
	"context"
	"sync"
	"time"

	"github.com/resmoio/kubernetes-event-exporter/pkg/metrics"
	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var startUpTime = time.Now()

type EventHandler func(event *EnhancedEvent)

type EventWatcher struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	informer            cache.SharedInformer
	stopper             chan struct{}
	objectMetadataCache ObjectMetadataProvider
	omitLookup          bool
	fn                  EventHandler
	maxEventAgeSeconds  time.Duration
	metricsStore        *metrics.Store
	dynamicClient       *dynamic.DynamicClient
	clientset           *kubernetes.Clientset
	watcher             watch.Interface
}

func NewEventWatcher(config *rest.Config, namespace string, MaxEventAgeSeconds int64, metricsStore *metrics.Store, fn EventHandler, omitLookup bool, cacheSize int) *EventWatcher {
	clientset := kubernetes.NewForConfigOrDie(config)
	//factory := informers.NewSharedInformerFactoryWithOptions(clientset, 0, informers.WithNamespace(namespace))
	//informer := factory.Core().V1().Events().Informer()

	ctx, cancel := context.WithCancel(context.Background())

	w, err := clientset.EventsV1().Events(namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		cancel()
		return nil
	}

	watcher := &EventWatcher{
		ctx:     ctx,
		cancel:  cancel,
		watcher: w,
		//informer:            informer,
		stopper:             make(chan struct{}),
		objectMetadataCache: NewObjectMetadataProvider(cacheSize),
		omitLookup:          omitLookup,
		fn:                  fn,
		maxEventAgeSeconds:  time.Second * time.Duration(MaxEventAgeSeconds),
		metricsStore:        metricsStore,
		dynamicClient:       dynamic.NewForConfigOrDie(config),
		clientset:           clientset,
	}

	//informer.AddEventHandler(watcher)
	//informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
	//	watcher.metricsStore.WatchErrors.Inc()
	//})

	return watcher
}

func (e *EventWatcher) loopHandle() {
	for {
		select {
		case evt := <-e.watcher.ResultChan():
			e.OnAdd(evt.Object)
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

	//if e.omitLookup {
	ev.InvolvedObject.ObjectReference = *event.InvolvedObject.DeepCopy()
	//} else {
	//	objectMetadata, err := e.objectMetadataCache.GetObjectMetadata(&event.InvolvedObject, e.clientset, e.dynamicClient, e.metricsStore)
	//	if err != nil {
	//		if errors.IsNotFound(err) {
	//			ev.InvolvedObject.Deleted = true
	//			log.Error().Err(err).Msg("Object not found, likely deleted")
	//		} else {
	//			log.Error().Err(err).Msg("Failed to get object metadata")
	//		}
	//		ev.InvolvedObject.ObjectReference = *event.InvolvedObject.DeepCopy()
	//	} else {
	//		ev.InvolvedObject.Labels = objectMetadata.Labels
	//		ev.InvolvedObject.Annotations = objectMetadata.Annotations
	//		ev.InvolvedObject.OwnerReferences = objectMetadata.OwnerReferences
	//		ev.InvolvedObject.ObjectReference = *event.InvolvedObject.DeepCopy()
	//		ev.InvolvedObject.Deleted = objectMetadata.Deleted
	//	}
	//}

	e.fn(ev)
}

func (e *EventWatcher) OnDelete(obj interface{}) {
	// Ignore deletes
}

func (e *EventWatcher) Start() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.loopHandle()
	}()
}

func (e *EventWatcher) Stop() {
	close(e.stopper)
	e.cancel()
	e.wg.Wait()
}

func (e *EventWatcher) setStartUpTime(time time.Time) {
	startUpTime = time
}
