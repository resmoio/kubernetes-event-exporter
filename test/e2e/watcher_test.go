//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/mustafaakin/kubernetes-event-exporter/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// hasReason returns a predicate matching events by namespace + reason.
func hasReason(namespace, reason string) func(*kube.EnhancedEvent) bool {
	return func(ev *kube.EnhancedEvent) bool {
		return ev.Namespace == namespace && ev.Reason == reason
	}
}

// TestCoreV1EventDelivered verifies that an Event created through the legacy
// core/v1 API is delivered to the watcher's handler.
func TestCoreV1EventDelivered(t *testing.T) {
	ns := createNamespace(t, "kee-corev1")
	c := startWatcher(t, ns, 3600, true, false)

	const reason = "CoreV1Probe"
	createCoreV1Event(t, ns, reason, time.Now())

	eventually(t, 30*time.Second, "core/v1 event to be delivered", func() bool {
		return len(c.find(hasReason(ns, reason))) > 0
	})
}

// TestEventsV1EventDelivered verifies that an Event created through the
// events.k8s.io/v1 API is also delivered. This is the regression guard for the
// recurring "events missing on newer Kubernetes versions" reports: the watcher
// subscribes to core/v1 Events, and the API server is expected to surface
// events.k8s.io/v1 objects through that same watch.
func TestEventsV1EventDelivered(t *testing.T) {
	ns := createNamespace(t, "kee-eventsv1")
	c := startWatcher(t, ns, 3600, true, false)

	const reason = "EventsV1Probe"
	createEventsV1Event(t, ns, reason, time.Now())

	eventually(t, 30*time.Second, "events.k8s.io/v1 event to be delivered", func() bool {
		return len(c.find(hasReason(ns, reason))) > 0
	})
}

// TestInvolvedObjectEnrichment verifies that when omitLookup is disabled the
// watcher enriches the event with the involved object's labels.
func TestInvolvedObjectEnrichment(t *testing.T) {
	ns := createNamespace(t, "kee-enrich")
	ctx := context.Background()

	cm, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "target-",
			Labels:       map[string]string{"e2e-label": "present"},
		},
		Data: map[string]string{"k": "v"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	c := startWatcher(t, ns, 3600, false /* omitLookup */, false)

	const reason = "EnrichProbe"
	ev := baseCoreV1Event(ns, reason, time.Now())
	ev.InvolvedObject = corev1.ObjectReference{
		Kind:       "ConfigMap",
		Namespace:  ns,
		Name:       cm.Name,
		UID:        cm.UID,
		APIVersion: "v1",
	}
	if _, err := clientset.CoreV1().Events(ns).Create(ctx, ev, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	eventually(t, 30*time.Second, "event enriched with involved object labels", func() bool {
		for _, e := range c.find(hasReason(ns, reason)) {
			if e.InvolvedObject.Labels["e2e-label"] == "present" {
				return true
			}
		}
		return false
	})
}

// TestRealPodFailureEvents verifies the full path against a kubelet/scheduler:
// a pod with an unresolvable image produces real failure events that flow
// through the watcher.
func TestRealPodFailureEvents(t *testing.T) {
	ns := createNamespace(t, "kee-podfail")
	c := startWatcher(t, ns, 3600, true, false)

	ctx := context.Background()
	pod, err := clientset.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "failer-"},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:  "c",
				Image: "registry.invalid/does-not-exist:nope",
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pod: %v", err)
	}

	forPod := func(ev *kube.EnhancedEvent) bool {
		return ev.InvolvedObject.Kind == "Pod" && ev.InvolvedObject.Name == pod.Name
	}

	// The scheduler emits a "Scheduled" event within seconds; this proves the
	// real watch pipeline works end to end.
	eventually(t, 60*time.Second, "any event for the failing pod", func() bool {
		return len(c.find(forPod)) > 0
	})

	// And the kubelet should report an image pull failure shortly after.
	eventually(t, 120*time.Second, "an image pull failure event for the pod", func() bool {
		for _, ev := range c.find(forPod) {
			switch ev.Reason {
			case "Failed", "ErrImagePull", "ImagePullBackOff", "BackOff":
				return true
			}
		}
		return false
	})
}

// TestMaxEventAgeDiscardsOldEvents verifies that events older than
// maxEventAgeSeconds are dropped while fresh ones are still delivered.
func TestMaxEventAgeDiscardsOldEvents(t *testing.T) {
	ns := createNamespace(t, "kee-age")
	c := startWatcher(t, ns, 120 /* maxEventAgeSeconds */, true, false)

	const oldReason = "TooOldProbe"
	const freshReason = "FreshProbe"
	createCoreV1Event(t, ns, oldReason, time.Now().Add(-1*time.Hour))
	createCoreV1Event(t, ns, freshReason, time.Now())

	// The fresh event must arrive...
	eventually(t, 30*time.Second, "fresh event delivered", func() bool {
		return len(c.find(hasReason(ns, freshReason))) > 0
	})
	// ...and once it has, the old one must not have been delivered.
	if got := c.find(hasReason(ns, oldReason)); len(got) > 0 {
		t.Fatalf("expected old event to be discarded, but it was delivered (%d times)", len(got))
	}
}

// TestNamespaceScoping verifies that a namespace-scoped watcher only receives
// events from its own namespace.
func TestNamespaceScoping(t *testing.T) {
	watched := createNamespace(t, "kee-watched")
	other := createNamespace(t, "kee-other")
	c := startWatcher(t, watched, 3600, true, false)

	const reason = "ScopeProbe"
	createCoreV1Event(t, other, reason, time.Now())
	createCoreV1Event(t, watched, reason, time.Now())

	// The in-scope event proves the watcher is live.
	eventually(t, 30*time.Second, "in-scope event delivered", func() bool {
		return len(c.find(hasReason(watched, reason))) > 0
	})
	// The out-of-scope event must never appear.
	if got := c.find(hasReason(other, reason)); len(got) > 0 {
		t.Fatalf("watcher scoped to %q received %d events from %q", watched, len(got), other)
	}
}

// TestRecurringEventReEmittedWhenReportUpdates verifies that, with
// reportUpdates enabled, a recurring event (Kubernetes bumps Count on the same
// object) is delivered again rather than only on its first occurrence.
func TestRecurringEventReEmittedWhenReportUpdates(t *testing.T) {
	ns := createNamespace(t, "kee-update")
	c := startWatcher(t, ns, 3600, true, true /* reportUpdates */)

	ctx := context.Background()
	const reason = "RecurProbe"
	ev := baseCoreV1Event(ns, reason, time.Now())
	ev.Count = 1
	created, err := clientset.CoreV1().Events(ns).Create(ctx, ev, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	eventually(t, 30*time.Second, "initial occurrence delivered", func() bool {
		return len(c.find(hasReason(ns, reason))) >= 1
	})

	// Simulate a recurrence the way the API server does: same object, higher
	// count and a fresh last-seen timestamp.
	created.Count = 2
	created.LastTimestamp = metav1.NewTime(time.Now())
	if _, err := clientset.CoreV1().Events(ns).Update(ctx, created, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	eventually(t, 30*time.Second, "recurrence re-emitted", func() bool {
		for _, e := range c.find(hasReason(ns, reason)) {
			if e.Count >= 2 {
				return true
			}
		}
		return false
	})
}

// TestRecurringEventIgnoredByDefault verifies the default behaviour: event
// updates are not re-emitted, so a recurrence is reported only once.
func TestRecurringEventIgnoredByDefault(t *testing.T) {
	ns := createNamespace(t, "kee-noupdate")
	c := startWatcher(t, ns, 3600, true, false /* reportUpdates */)

	ctx := context.Background()
	const reason = "NoRecurProbe"
	ev := baseCoreV1Event(ns, reason, time.Now())
	ev.Count = 1
	created, err := clientset.CoreV1().Events(ns).Create(ctx, ev, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	eventually(t, 30*time.Second, "initial occurrence delivered", func() bool {
		return len(c.find(hasReason(ns, reason))) >= 1
	})

	created.Count = 2
	created.LastTimestamp = metav1.NewTime(time.Now())
	if _, err := clientset.CoreV1().Events(ns).Update(ctx, created, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	// Give the watcher time to (not) process the update, then assert the
	// recurrence was never delivered.
	time.Sleep(3 * time.Second)
	for _, e := range c.find(hasReason(ns, reason)) {
		if e.Count >= 2 {
			t.Fatalf("recurrence was delivered despite reportUpdates being disabled")
		}
	}
}

// --- event builders ---

func baseCoreV1Event(namespace, reason string, ts time.Time) *corev1.Event {
	mt := metav1.NewTime(ts)
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "e2e-",
			Namespace:    namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			// A namespaced reference: the API server requires
			// involvedObject.namespace to equal the event's namespace.
			Kind:       "Pod",
			Namespace:  namespace,
			Name:       "synthetic-" + reason,
			APIVersion: "v1",
		},
		Reason:         reason,
		Message:        "synthetic e2e event: " + reason,
		Type:           corev1.EventTypeNormal,
		Source:         corev1.EventSource{Component: "e2e-test"},
		FirstTimestamp: mt,
		LastTimestamp:  mt,
		Count:          1,
	}
}

func createCoreV1Event(t *testing.T, namespace, reason string, ts time.Time) {
	t.Helper()
	_, err := clientset.CoreV1().Events(namespace).Create(
		context.Background(), baseCoreV1Event(namespace, reason, ts), metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("create core/v1 event: %v", err)
	}
}

func createEventsV1Event(t *testing.T, namespace, reason string, ts time.Time) {
	t.Helper()
	_, err := clientset.EventsV1().Events(namespace).Create(context.Background(), &eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "e2e-",
			Namespace:    namespace,
		},
		EventTime:           metav1.NewMicroTime(ts),
		ReportingController: "e2e-test",
		ReportingInstance:   "e2e-test-0",
		Action:              "Probing",
		Reason:              reason,
		Regarding: corev1.ObjectReference{
			Kind:       "Pod",
			Namespace:  namespace,
			Name:       "synthetic-" + reason,
			APIVersion: "v1",
		},
		Note: "synthetic e2e event via events.k8s.io/v1: " + reason,
		Type: corev1.EventTypeNormal,
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create events.k8s.io/v1 event: %v", err)
	}
}
