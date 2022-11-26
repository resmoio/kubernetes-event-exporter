package kube

import (
	"bytes"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEventWatcher_EventAge_whenEventCreatedBeforeStartup(t *testing.T) {
	// should not discard events as old as 300s=5m
	var MaxEventAgeSeconds int64 = 300
	ew := NewMockEventWatcher(MaxEventAgeSeconds)
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	// event is 3m before stratup time -> expect silently dropped
	startup := time.Now().Add(-10 * time.Minute)
	ew.SetStartUpTime(startup)
	event1 := corev1.Event{
		LastTimestamp: metav1.Time{Time: startup.Add(-3 * time.Minute)},
	}

	// event is 3m before stratup time -> expect silently dropped
	assert.True(t, ew.isEventDiscarded(&event1))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event1)
	assert.NotContains(t, output.String(), "Received event")

	event2 := corev1.Event{
		EventTime: metav1.MicroTime{Time: startup.Add(-3 * time.Minute)},
	}

	assert.True(t, ew.isEventDiscarded(&event2))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event2)
	assert.NotContains(t, output.String(), "Received event")

	// event is 3m before stratup time -> expect silently dropped
	event3 := corev1.Event{
		LastTimestamp: metav1.Time{Time: startup.Add(-3 * time.Minute)},
		EventTime:     metav1.MicroTime{Time: startup.Add(-3 * time.Minute)},
	}

	assert.True(t, ew.isEventDiscarded(&event3))
	assert.NotContains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event3)
	assert.NotContains(t, output.String(), "Received event")
}

func TestEventWatcher_EventAge_whenEventCreatedAfterStartupAndBeforeMaxAge(t *testing.T) {
	// should not discard events as old as 300s=5m
	var MaxEventAgeSeconds int64 = 300
	ew := NewMockEventWatcher(MaxEventAgeSeconds)
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	startup := time.Now().Add(-10 * time.Minute)
	ew.SetStartUpTime(startup)
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
}

func TestEventWatcher_EventAge_whenEventCreatedAfterStartupAndAfterMaxAge(t *testing.T) {
	// should not discard events as old as 300s=5m
	var MaxEventAgeSeconds int64 = 300
	ew := NewMockEventWatcher(MaxEventAgeSeconds)
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	// event is 3m after stratup time (and 2m after max age) -> expect dropped with warn
	startup := time.Now().Add(-10 * time.Minute)
	ew.SetStartUpTime(startup)
	event1 := corev1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "event1"},
		LastTimestamp: metav1.Time{Time: startup.Add(3 * time.Minute)},
	}
	assert.True(t, ew.isEventDiscarded(&event1))
	assert.Contains(t, output.String(), "event1")
	assert.Contains(t, output.String(), "Event discarded as being older then maxEventAgeSeconds")
	ew.onEvent(&event1)
	assert.NotContains(t, output.String(), "Received event")

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
}
