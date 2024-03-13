package throttler

import (
	"errors"
	"time"
)

type Throttler struct {
	n            int
	completed    bool
	currentDelay time.Duration
	minDelay     time.Duration
	factor       float32
	maxDelay     time.Duration
	recovery     time.Duration
	lastCall     time.Time
	lastSlowdown time.Time
	err          error
}

func NewThrottler(minDelay time.Duration) Throttler {
	return Throttler{
		currentDelay: minDelay,
		minDelay:     minDelay,
		maxDelay:     60.0 * time.Second,
		recovery:     60.0 * time.Second,
		factor:       2.0,
	}
}

func (t *Throttler) Wait() bool {
	if t.completed {
		// don't wait when completed, abort
		// also presumes that err == nil
		t.completed = false
		return false
	}
	if t.err != nil {
		// don't wait on errors, abort
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
		t.err = errors.New("wait too long")
	}
	return true
}

func (t *Throttler) Error() error {
	err := t.err
	t.err = nil
	t.completed = false
	return err
}

func (t *Throttler) Complete() {
	t.tryRecover()
	t.n += 1
	t.lastCall = time.Now()
	t.completed = true
	// completed task have no errors
	t.err = nil
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
	if !t.lastSlowdown.IsZero() && time.Since(t.lastSlowdown) > t.recovery {
		t.currentDelay = max(t.minDelay, t.currentDelay/time.Duration(t.factor))
	}
}
