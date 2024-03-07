package main

import "time"

type Throttler struct {
	name         string
	n            int
	currentDelay time.Duration
	minDelay     time.Duration
	factor       float32
	maxDelay     time.Duration
	recovery     time.Duration
	cooldown     time.Duration
	lastCall     time.Time
	lastError    time.Time
}

func NewThrottler(name string, minDelay time.Duration) *Throttler {
	return &Throttler{
		name:     name,
		currentDelay: minDelay,
		minDelay: minDelay,
		maxDelay: 30.0 * time.Second,
		recovery: 30.0 * time.Second,
		cooldown: 60.0 * time.Second,
		factor:   2.0,
	}
}

func (r *Throttler) Wait() bool {
	r.touch()
	if r.n == 0 {
		return true
	}
	if r.currentDelay >= r.maxDelay {
		return false
	}
	time.Sleep(r.currentDelay)
	return true
}

func (r *Throttler) touch() {
	if time.Since(r.lastCall) > r.cooldown {
		r.n = 0
		r.currentDelay = r.minDelay
		r.lastCall = time.Time{}
		r.lastError = time.Time{}
	}
}

func (r *Throttler) Ok() {
	r.n += 1
	r.lastCall = time.Now()
	if !r.lastError.IsZero() && time.Since(r.lastError) > r.recovery {
		r.currentDelay = max(r.minDelay, r.currentDelay/time.Duration(r.factor))
	}
}

func (r *Throttler) TooManyRequests() {
	r.n += 1
	r.lastCall = time.Now()
	r.lastError = time.Now()
	r.currentDelay = min(r.maxDelay, r.currentDelay * time.Duration(r.factor))
}

func (r *Throttler) Error(err error) {
	r.n += 1
	r.lastCall = time.Now()
	r.lastError = time.Now()
	r.currentDelay = min(r.maxDelay, r.currentDelay * time.Duration(r.factor))
}
