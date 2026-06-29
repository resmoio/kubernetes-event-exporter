package batch

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// recorder is a thread-safe collector for the batches handed to the Writer's
// callback. The callback runs in the Writer's own goroutine, so tests that read
// results before Stop() (e.g. the interval-based ones) must synchronise access.
type recorder struct {
	mu      sync.Mutex
	batches [][]interface{}
}

func (r *recorder) record(items []interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, items)
}

func (r *recorder) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.batches)
}

func (r *recorder) get(i int) []interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.batches[i]
}

// allSucceed is a callback factory that records each batch and reports success
// for every item.
func allSucceed(r *recorder) Callback {
	return func(ctx context.Context, items []interface{}) []bool {
		resp := make([]bool, len(items))
		for idx := range resp {
			resp[idx] = true
		}
		r.record(items)
		return resp
	}
}

func TestSimpleWriter(t *testing.T) {
	cfg := WriterConfig{
		BatchSize:  10,
		MaxRetries: 3,
		Interval:   time.Second * 2,
	}

	r := &recorder{}
	w := NewWriter(cfg, allSucceed(r))

	w.Start()
	w.Submit(1, 2, 3, 4, 5, 6, 7)
	assert.Equal(t, 0, r.len())
	w.Stop()

	assert.Equal(t, 1, r.len())
	got := r.get(0)
	assert.Len(t, got, 7)
	for i := 0; i < 7; i++ {
		assert.Equal(t, got[i], i+1)
	}
}

func TestCorrectnessManyTimes(t *testing.T) {
	// Surely this is not the proper way to do it but anyways
	for i := 0; i < 10000; i++ {
		TestSimpleWriter(t)
	}
}

func TestLargerThanBatchSize(t *testing.T) {
	cfg := WriterConfig{
		BatchSize:  3,
		MaxRetries: 3,
		Interval:   time.Second * 2,
	}

	r := &recorder{}
	w := NewWriter(cfg, allSucceed(r))

	w.Start()
	w.Submit(1, 2, 3, 4, 5, 6, 7)
	w.Stop()

	assert.Equal(t, 3, r.len())
	assert.Equal(t, r.get(0), []interface{}{1, 2, 3})
	assert.Equal(t, r.get(1), []interface{}{4, 5, 6})
	assert.Equal(t, r.get(2), []interface{}{7})
}

func TestSimpleInterval(t *testing.T) {
	cfg := WriterConfig{
		BatchSize:  5,
		MaxRetries: 3,
		Interval:   time.Millisecond * 20,
	}

	r := &recorder{}
	w := NewWriter(cfg, allSucceed(r))

	w.Start()
	w.Submit(1, 2)
	time.Sleep(time.Millisecond * 5)
	assert.Equal(t, 0, r.len())

	time.Sleep(time.Millisecond * 50)
	assert.Equal(t, 1, r.len())
	assert.Equal(t, r.get(0), []interface{}{1, 2})

	w.Stop()
	assert.Equal(t, 1, r.len())
}

func TestIntervalComplex(t *testing.T) {
	cfg := WriterConfig{
		BatchSize:  5,
		MaxRetries: 3,
		Interval:   time.Millisecond * 20,
	}

	r := &recorder{}
	w := NewWriter(cfg, allSucceed(r))

	w.Start()
	w.Submit(1, 2)
	time.Sleep(time.Millisecond * 5)
	w.Submit(3, 4)
	assert.Equal(t, 0, r.len())

	time.Sleep(time.Millisecond * 50)
	assert.Equal(t, 1, r.len())
	assert.Equal(t, r.get(0), []interface{}{1, 2, 3, 4})

	w.Stop()
	assert.Equal(t, 1, r.len())
}

func TestIntervalComplexAfterFlush(t *testing.T) {
	cfg := WriterConfig{
		BatchSize:  5,
		MaxRetries: 3,
		Interval:   time.Millisecond * 20,
	}

	r := &recorder{}
	w := NewWriter(cfg, allSucceed(r))

	w.Start()
	w.Submit(1, 2)
	time.Sleep(time.Millisecond * 5)
	w.Submit(3, 4)
	assert.Equal(t, 0, r.len())

	time.Sleep(time.Millisecond * 50)
	assert.Equal(t, 1, r.len())
	assert.Equal(t, r.get(0), []interface{}{1, 2, 3, 4})

	w.Submit(5, 6, 7)
	w.Stop()

	assert.Equal(t, 2, r.len())
	assert.Equal(t, r.get(1), []interface{}{5, 6, 7})
}

func TestRetry(t *testing.T) {
	cfg := WriterConfig{
		BatchSize:  5,
		MaxRetries: 3,
		Interval:   time.Millisecond * 10,
	}

	r := &recorder{}
	w := NewWriter(cfg, func(ctx context.Context, items []interface{}) []bool {
		resp := make([]bool, len(items))
		for idx := range resp {
			resp[idx] = items[idx] != 2
		}
		r.record(items)
		return resp
	})

	w.Start()
	w.Submit(1, 2, 3)
	assert.Equal(t, 0, r.len())

	time.Sleep(time.Millisecond * 200)
	w.Stop()
	assert.Equal(t, 4, r.len())

	assert.Equal(t, r.get(0), []interface{}{1, 2, 3})
	assert.Equal(t, r.get(1), []interface{}{2})
	assert.Equal(t, r.get(2), []interface{}{2})
	assert.Equal(t, r.get(3), []interface{}{2})
}
