package kube

import (
	"bytes"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/mustafaakin/kubernetes-event-exporter/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type mockObjectMetadataProvider struct {
	cache      *lru.ARCCache
	objDeleted bool
}

func newMockObjectMetadataProvider() ObjectMetadataProvider {
	cache, err := lru.NewARC(1024)
	if err != nil {
		panic("cannot init cache: " + err.Error())
	}

	cache.Add("test", ObjectMetadata{
		Annotations: map[string]string{"test": "test"},
		Labels:      map[string]string{"test": "test"},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "testAPI",
				Kind:       "testKind",
				Name:       "testOwner",
				UID:        "testOwner",
			},
		},
	})

	var o ObjectMetadataProvider = &mockObjectMetadataProvider{
		cache:      cache,
		objDeleted: false,
	}

	return o
}

func (o *mockObjectMetadataProvider) GetObjectMetadata(reference *corev1.ObjectReference, clientset *kubernetes.Clientset, dynClient dynamic.Interface, metricsStore *metrics.Store) (ObjectMetadata, error) {
	if o.objDeleted {
		return ObjectMetadata{}, errors.NewNotFound(schema.GroupResource{}, "")
	}

	val, _ := o.cache.Get("test")
	return val.(ObjectMetadata), nil
}

var _ ObjectMetadataProvider = &mockObjectMetadataProvider{}

func newMockEventWatcher(MaxEventAgeSeconds int64, metricsStore *metrics.Store) *EventWatcher {
	watcher := &EventWatcher{
		objectMetadataCache: newMockObjectMetadataProvider(),
		maxEventAgeSeconds:  time.Second * time.Duration(MaxEventAgeSeconds),
		fn:                  func(event *EnhancedEvent) {},
		metricsStore:        metricsStore,
	}
	return watcher
}

func TestEventWatcher_EventAge_whenEventCreatedBeforeStartup(t *testing.T) {
	// should not discard events as old as 300s=5m
	var MaxEventAgeSeconds int64 = 300
	metricsStore := metrics.NewMetricsStore("test_")
	ew := newMockEventWatcher(MaxEventAgeSeconds, metricsStore)
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	// event is 3m before stratup time -> expect silently dropped
	startup := time.Now().Add(-10 * time.Minute)
	ew.setStartUpTime(startup)
	event1 := corev1.Event{
		LastTimestamp: metav1.Time{Time: startup.Add(-3 * time.Minute)},
	}

	// event is 3m before stratup time -> expect silently dropped
	assert.True(t, ew.isEventDiscarded(&event1))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event1)
	assert.NotContains(t, output.String(), "Received event")
	assert.Equal(t, float64(0), testutil.ToFloat64(metricsStore.EventsProcessed))

	event2 := corev1.Event{
		EventTime: metav1.MicroTime{Time: startup.Add(-3 * time.Minute)},
	}

	assert.True(t, ew.isEventDiscarded(&event2))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event2)
	assert.NotContains(t, output.String(), "Received event")
	assert.Equal(t, float64(0), testutil.ToFloat64(metricsStore.EventsProcessed))

	// event is 3m before stratup time -> expect silently dropped
	event3 := corev1.Event{
		LastTimestamp: metav1.Time{Time: startup.Add(-3 * time.Minute)},
		EventTime:     metav1.MicroTime{Time: startup.Add(-3 * time.Minute)},
	}

	assert.True(t, ew.isEventDiscarded(&event3))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event3)
	assert.NotContains(t, output.String(), "Received event")
	assert.Equal(t, float64(0), testutil.ToFloat64(metricsStore.EventsProcessed))

	metrics.DestroyMetricsStore(metricsStore)
}

func TestEventWatcher_EventAge_whenEventCreatedAfterStartupAndBeforeMaxAge(t *testing.T) {
	// should not discard events as old as 300s=5m
	var MaxEventAgeSeconds int64 = 300
	metricsStore := metrics.NewMetricsStore("test_")
	ew := newMockEventWatcher(MaxEventAgeSeconds, metricsStore)
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	startup := time.Now().Add(-10 * time.Minute)
	ew.setStartUpTime(startup)
	// event is 8m after stratup time (2m in max age) -> expect processed
	event1 := corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			UID:  "test",
			Name: "test-1",
		},
		LastTimestamp: metav1.Time{Time: startup.Add(8 * time.Minute)},
	}

	assert.False(t, ew.isEventDiscarded(&event1))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event1)
	assert.Contains(t, output.String(), "test-1")
	assert.Contains(t, output.String(), "Received event")
	assert.Equal(t, float64(1), testutil.ToFloat64(metricsStore.EventsProcessed))

	// event is 8m after stratup time (2m in max age) -> expect processed
	event2 := corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			UID:  "test",
			Name: "test-2",
		},
		EventTime: metav1.MicroTime{Time: startup.Add(8 * time.Minute)},
	}

	assert.False(t, ew.isEventDiscarded(&event2))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event2)
	assert.Contains(t, output.String(), "test-2")
	assert.Contains(t, output.String(), "Received event")
	assert.Equal(t, float64(2), testutil.ToFloat64(metricsStore.EventsProcessed))

	// event is 8m after stratup time (2m in max age) -> expect processed
	event3 := corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			UID:  "test",
			Name: "test-3",
		},
		LastTimestamp: metav1.Time{Time: startup.Add(8 * time.Minute)},
		EventTime:     metav1.MicroTime{Time: startup.Add(8 * time.Minute)},
	}

	assert.False(t, ew.isEventDiscarded(&event3))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event3)
	assert.Contains(t, output.String(), "test-3")
	assert.Contains(t, output.String(), "Received event")
	assert.Equal(t, float64(3), testutil.ToFloat64(metricsStore.EventsProcessed))

	metrics.DestroyMetricsStore(metricsStore)
}

func TestEventWatcher_EventAge_whenEventCreatedAfterStartupAndAfterMaxAge(t *testing.T) {
	// should not discard events as old as 300s=5m
	var MaxEventAgeSeconds int64 = 300
	metricsStore := metrics.NewMetricsStore("test_")
	ew := newMockEventWatcher(MaxEventAgeSeconds, metricsStore)
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	// event is 3m after stratup time (and 2m after max age) -> expect dropped with warn
	startup := time.Now().Add(-10 * time.Minute)
	ew.setStartUpTime(startup)
	event1 := corev1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "event1"},
		LastTimestamp: metav1.Time{Time: startup.Add(3 * time.Minute)},
	}
	assert.True(t, ew.isEventDiscarded(&event1))
	assert.Contains(t, output.String(), "event1")
	assert.Contains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event1)
	assert.NotContains(t, output.String(), "Received event")
	assert.Equal(t, float64(0), testutil.ToFloat64(metricsStore.EventsProcessed))

	// event is 3m after stratup time (and 2m after max age) -> expect dropped with warn
	event2 := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "event2"},
		EventTime:  metav1.MicroTime{Time: startup.Add(3 * time.Minute)},
	}

	assert.True(t, ew.isEventDiscarded(&event2))
	assert.Contains(t, output.String(), "event2")
	assert.Contains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event2)
	assert.NotContains(t, output.String(), "Received event")
	assert.Equal(t, float64(0), testutil.ToFloat64(metricsStore.EventsProcessed))

	// event is 3m after stratup time (and 2m after max age) -> expect dropped with warn
	event3 := corev1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "event3"},
		LastTimestamp: metav1.Time{Time: startup.Add(3 * time.Minute)},
		EventTime:     metav1.MicroTime{Time: startup.Add(3 * time.Minute)},
	}

	assert.True(t, ew.isEventDiscarded(&event3))
	assert.Contains(t, output.String(), "event3")
	assert.Contains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event3)
	assert.NotContains(t, output.String(), "Received event")
	assert.Equal(t, float64(0), testutil.ToFloat64(metricsStore.EventsProcessed))

	metrics.DestroyMetricsStore(metricsStore)
}

func TestOnEvent_WithObjectMetadata(t *testing.T) {
	metricsStore := metrics.NewMetricsStore("test_")
	defer metrics.DestroyMetricsStore(metricsStore)
	ew := newMockEventWatcher(300, metricsStore)

	event := EnhancedEvent{}
	ew.fn = func(e *EnhancedEvent) {
		event = *e
	}

	startup := time.Now().Add(-10 * time.Minute)
	ew.setStartUpTime(startup)
	event1 := corev1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "event1"},
		LastTimestamp: metav1.Time{Time: startup.Add(8 * time.Minute)},
		InvolvedObject: corev1.ObjectReference{
			UID:  "test",
			Name: "test-1",
		},
	}
	ew.onEvent(&event1)

	require.Equal(t, types.UID("test"), event.InvolvedObject.UID)
	require.Equal(t, "test-1", event.InvolvedObject.Name)
	require.Equal(t, map[string]string{"test": "test"}, event.InvolvedObject.Annotations)
	require.Equal(t, map[string]string{"test": "test"}, event.InvolvedObject.Labels)
	require.Equal(t, []metav1.OwnerReference{
		{
			APIVersion: "testAPI",
			Kind:       "testKind",
			Name:       "testOwner",
			UID:        "testOwner",
		},
	}, event.InvolvedObject.OwnerReferences)
}

func TestOnEvent_DeletedObjects(t *testing.T) {
	metricsStore := metrics.NewMetricsStore("test_")
	defer metrics.DestroyMetricsStore(metricsStore)
	ew := newMockEventWatcher(300, metricsStore)
	ew.objectMetadataCache.(*mockObjectMetadataProvider).objDeleted = true

	event := EnhancedEvent{}
	ew.fn = func(e *EnhancedEvent) {
		event = *e
	}

	startup := time.Now().Add(-10 * time.Minute)
	ew.setStartUpTime(startup)
	event1 := corev1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "event1"},
		LastTimestamp: metav1.Time{Time: startup.Add(8 * time.Minute)},
		InvolvedObject: corev1.ObjectReference{
			UID:  "test",
			Name: "test-1",
		},
	}

	ew.onEvent(&event1)

	require.Equal(t, types.UID("test"), event.InvolvedObject.UID)
	require.Equal(t, "test-1", event.InvolvedObject.Name)
	require.Equal(t, true, event.InvolvedObject.Deleted)
	require.Equal(t, map[string]string(nil), event.InvolvedObject.Annotations)
	require.Equal(t, map[string]string(nil), event.InvolvedObject.Labels)
	require.Equal(t, []metav1.OwnerReference(nil), event.InvolvedObject.OwnerReferences)
}
