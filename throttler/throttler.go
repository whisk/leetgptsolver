package throttler

import (
	"time"
)

type Throttler struct {
	n            int
	completed    bool
	timeout      bool
	currentDelay time.Duration
	minDelay     time.Duration
	factor       float32
	maxDelay     time.Duration
	lastCall     time.Time
	lastSlowdown time.Time
}

func NewThrottler(minDelay, maxDelay time.Duration) Throttler {
	return Throttler{
		currentDelay: minDelay,
		minDelay:     minDelay,
		maxDelay:     maxDelay,
		factor:       2.0,
	}
}

func (t *Throttler) Wait() bool {
	if t.completed || t.timeout {
		// don't wait when completed, abort
		// also don't wait when we are already timed out
		t.completed = false
		t.timeout = false
		return false
	}
	if t.n == 0 {
		// no wait on the first attempt, proceed
		return true
	}
	time.Sleep(t.currentDelay)
	if t.currentDelay > t.maxDelay {
		// we waited for too long, flag the error
		// we won't wait again unless task is completed on the very next run
		t.timeout = true
	}
	return true
}

// reserved for future use
func (t *Throttler) Error() error {
	t.completed = false
	t.timeout = false
	return nil
}

func (t *Throttler) Complete() {
	t.tryRecover()
	t.n += 1
	t.lastCall = time.Now()
	t.completed = true
	t.timeout = false
}

func (t *Throttler) Again() {
	t.n += 1
	t.lastCall = time.Now()
}

// failure that indicates that we should reduce the rate
func (t *Throttler) Slower() {
	t.n += 1
	t.lastCall = time.Now()
	t.lastSlowdown = time.Now()
	t.currentDelay = min(t.maxDelay+1*time.Millisecond, t.currentDelay*time.Duration(t.factor))
}

func (t *Throttler) tryRecover() {
	if !t.lastSlowdown.IsZero() && time.Since(t.lastSlowdown) > t.maxDelay {
		t.currentDelay = max(t.minDelay, t.currentDelay/time.Duration(t.factor))
	}
}
