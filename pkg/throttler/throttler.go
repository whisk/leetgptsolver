package throttler

import (
	"time"
)

type Throttler interface {
	Ready()
	Wait() bool
	Touch()
	Slowdown()
}

type SimpleThrottler struct {
	n            int
	currentDelay time.Duration
	minDelay     time.Duration
	factor       float32
	maxDelay     time.Duration
	lastSlowdown time.Time
}

func NewSimpleThrottler(minDelay, maxDelay time.Duration) *SimpleThrottler {
	return &SimpleThrottler{
		currentDelay: minDelay,
		minDelay:     minDelay,
		maxDelay:     maxDelay,
		factor:       2.0,
		lastSlowdown: time.Time{},
	}
}

func (t *SimpleThrottler) Ready() {
	t.tryRecover()
}

func (t *SimpleThrottler) Wait() bool {
	t.n += 1
	if t.n == 1 {
		// no wait on the very first attempt
		return true
	}
	time.Sleep(t.currentDelay)
	return true
}

func (t *SimpleThrottler) Touch() {
	t.tryRecover()
}

func (t *SimpleThrottler) Slowdown() {
	t.lastSlowdown = time.Now()
	t.currentDelay = min(t.maxDelay, t.currentDelay*time.Duration(t.factor))
}

func (t *SimpleThrottler) tryRecover() {
	if !t.lastSlowdown.IsZero() && time.Since(t.lastSlowdown) > t.maxDelay {
		t.currentDelay = max(t.minDelay, t.currentDelay/time.Duration(t.factor))
	}
}
