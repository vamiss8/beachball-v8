package main

import (
	"sync"
	"testing"
	"time"
)

func TestHistogramPercentiles(t *testing.T) {
	h := newHistogram(time.Millisecond, 100*time.Millisecond)

	// ninety fast samples and ten slow ones
	for i := 0; i < 90; i++ {
		h.record(2 * time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		h.record(40 * time.Millisecond)
	}

	if got := h.percentile(0.5); got != 3*time.Millisecond {
		t.Fatalf("p50 = %v, want 3ms (upper edge of the 2ms bucket)", got)
	}
	// the bucket edge would say 41ms, but no sample was ever slower than 40
	if got := h.percentile(0.99); got != 40*time.Millisecond {
		t.Fatalf("p99 = %v, want 40ms", got)
	}
	if got := time.Duration(h.max.Load()); got != 40*time.Millisecond {
		t.Fatalf("max = %v, want 40ms", got)
	}
}

func TestHistogramOverflowStillCountsAndKeepsTheMax(t *testing.T) {
	h := newHistogram(time.Millisecond, 10*time.Millisecond)
	h.record(time.Second)

	if h.total() != 1 {
		t.Fatalf("total = %d, want 1", h.total())
	}
	if got := h.percentile(0.99); got != time.Second {
		t.Fatalf("p99 = %v, want the one second maximum", got)
	}
}

func TestHistogramIsSafeUnderConcurrentWriters(t *testing.T) {
	h := newHistogram(time.Millisecond, 50*time.Millisecond)

	const writers, each = 32, 1000
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				h.record(time.Duration(w) * time.Millisecond)
			}
		}(w)
	}
	wg.Wait()

	// a lost increment would show up here; -race would flag the torn writes
	if got := h.total(); got != writers*each {
		t.Fatalf("total = %d, want %d", got, writers*each)
	}
	if got := time.Duration(h.max.Load()); got != (writers-1)*time.Millisecond {
		t.Fatalf("max = %v, want %v", got, (writers-1)*time.Millisecond)
	}
}

func TestHistogramPercentileNeverExceedsTheMaximum(t *testing.T) {
	// the loopback ping that first exposed this: every sample inside the
	// first bucket, so its upper edge sat above anything actually measured
	h := newHistogram(50*time.Microsecond, time.Millisecond)
	h.record(30 * time.Microsecond)
	h.record(20 * time.Microsecond)

	if got := h.percentile(0.5); got != 30*time.Microsecond {
		t.Fatalf("p50 = %v, want it capped at the 30us maximum", got)
	}
}
