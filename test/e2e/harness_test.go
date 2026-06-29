//go:build e2e

// Package e2e contains in-process integration tests that run the real event
// watcher and engine against a live Kubernetes cluster (typically a kind
// cluster spun up by CI across a version matrix). The tests are gated behind
// the `e2e` build tag so they never run as part of the normal `go test ./...`
// unit suite.
//
// Run locally against the current kube context with:
//
//	make e2e            # assumes a cluster is already reachable
//	make e2e-kind       # creates a throwaway kind cluster, runs, tears down
package e2e

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mustafaakin/kubernetes-event-exporter/pkg/kube"
	"github.com/mustafaakin/kubernetes-event-exporter/pkg/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Shared across all tests in the binary. Populated by TestMain.
var (
	restConfig *rest.Config
	clientset  *kubernetes.Clientset
)

// storeCounter guarantees a unique metric name prefix per watcher so the
// process-global Prometheus registry never sees a duplicate registration.
var storeCounter atomic.Int64

func TestMain(m *testing.M) {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cannot build kube config: %v\n", err)
		fmt.Fprintln(os.Stderr, "e2e: ensure a cluster is reachable (KUBECONFIG or in-cluster) before running with -tags e2e")
		os.Exit(1)
	}
	restConfig = cfg

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cannot build clientset: %v\n", err)
		os.Exit(1)
	}
	clientset = cs

	// Fail fast if the cluster is not actually reachable, with a clear message.
	ver, err := cs.Discovery().ServerVersion()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cluster not reachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "e2e: running against Kubernetes %s\n", ver.GitVersion)

	os.Exit(m.Run())
}

// loadConfig resolves a rest.Config from, in order: in-cluster config, then the
// standard client-go loading rules (which honour the KUBECONFIG env var and
// ~/.kube/config).
func loadConfig() (*rest.Config, error) {
	if c, err := rest.InClusterConfig(); err == nil {
		return c, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
}

// collector is a thread-safe kube.EventHandler that records every event the
// watcher hands it, so tests can assert on what was delivered.
type collector struct {
	mu     sync.Mutex
	events []*kube.EnhancedEvent
}

func (c *collector) handle(ev *kube.EnhancedEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

// find returns the events matching pred, taking a snapshot under the lock.
func (c *collector) find(pred func(*kube.EnhancedEvent) bool) []*kube.EnhancedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []*kube.EnhancedEvent
	for _, ev := range c.events {
		if pred(ev) {
			out = append(out, ev)
		}
	}
	return out
}

// newStore returns a metrics.Store with a per-test unique prefix and registers
// cleanup so the global Prometheus registry stays clean between tests.
func newStore(t *testing.T) *metrics.Store {
	t.Helper()
	prefix := fmt.Sprintf("e2e_%d_", storeCounter.Add(1))
	s := metrics.NewMetricsStore(prefix)
	t.Cleanup(func() { metrics.DestroyMetricsStore(s) })
	return s
}

// startWatcher constructs a real EventWatcher scoped to namespace, wires it to
// the returned collector, starts it, and registers Stop on cleanup.
func startWatcher(t *testing.T, namespace string, maxEventAgeSeconds int64, omitLookup, reportUpdates bool) *collector {
	t.Helper()
	c := &collector{}
	w := kube.NewEventWatcher(restConfig, namespace, maxEventAgeSeconds, newStore(t), c.handle, omitLookup, 1024, reportUpdates)
	w.Start()
	t.Cleanup(w.Stop)
	return c
}

// createNamespace creates a uniquely named namespace and deletes it on cleanup.
func createNamespace(t *testing.T, prefix string) string {
	t.Helper()
	ctx := context.Background()
	ns, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{GenerateName: prefix + "-"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	name := ns.Name
	t.Cleanup(func() {
		// Best effort; namespace teardown is async.
		_ = clientset.CoreV1().Namespaces().Delete(
			context.Background(), name, metav1.DeleteOptions{},
		)
	})
	return name
}

// eventually polls fn until it returns true or the timeout elapses.
func eventually(t *testing.T, timeout time.Duration, msg string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, msg)
}
