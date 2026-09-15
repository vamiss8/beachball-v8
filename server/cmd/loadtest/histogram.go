package main

import (
	"sync/atomic"
	"time"
)

// histogram counts durations into fixed-width buckets.
//
// every simulated player records into the same histogram at once, so it is
// built from atomics rather than a mutex: sixty samples a second from a
// thousand players is sixty thousand increments, and a lock taken for each of
// them would measure the load test's own contention instead of the server.
// fixed buckets also make percentiles exact to the bucket width without ever
// storing the samples themselves.
type histogram struct {
	width    time.Duration
	counts   []atomic.Uint64
	overflow atomic.Uint64
	max      atomic.Int64
}

func newHistogram(width, limit time.Duration) *histogram {
	return &histogram{
		width:  width,
		counts: make([]atomic.Uint64, int(limit/width)),
	}
}

func (h *histogram) record(d time.Duration) {
	if d < 0 {
		return
	}

	if i := int(d / h.width); i < len(h.counts) {
		h.counts[i].Add(1)
	} else {
		h.overflow.Add(1)
	}

	// compare-and-swap loop: two goroutines racing to raise the maximum must
	// not let the smaller one overwrite the larger
	for {
		cur := h.max.Load()
		if int64(d) <= cur || h.max.CompareAndSwap(cur, int64(d)) {
			return
		}
	}
}

func (h *histogram) total() uint64 {
	n := h.overflow.Load()
	for i := range h.counts {
		n += h.counts[i].Load()
	}
	return n
}

// percentile returns the upper edge of the bucket holding the p-th sample,
// with p between 0 and 1, never more than the largest sample actually seen.
// without that clamp a sub-bucket sample would report a p50 above its own
// maximum, which is a number no reader could make sense of.
func (h *histogram) percentile(p float64) time.Duration {
	n := h.total()
	if n == 0 {
		return 0
	}

	target := uint64(float64(n)*p + 0.5)
	if target == 0 {
		target = 1
	}

	most := time.Duration(h.max.Load())
	var seen uint64
	for i := range h.counts {
		seen += h.counts[i].Load()
		if seen >= target {
			return min(time.Duration(i+1)*h.width, most)
		}
	}
	return most
}
