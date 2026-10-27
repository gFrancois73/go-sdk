package go_sdk

import (
	"sync"
	"time"
)

// Clock is an interface for obtaining the current time.
// It exists so that tests can supply a deterministic fake clock
// instead of depending on wall-clock time. All rate decisions in
// this package are driven by a Clock, never by time.Now directly.
type Clock interface {
	Now() time.Time
}

// SystemClock is the default Clock implementation. It delegates to
// time.Now. It is safe for concurrent use because time.Now is.
type SystemClock struct{}

// Now returns the real current time.
func (SystemClock) Now() time.Time { return time.Now() }

// NewSystemClock returns a Clock backed by the system clock.
func NewSystemClock() Clock { return SystemClock{} }

// fakeClock is an in-memory Clock used only by tests. It is safe
// for concurrent use because every method takes a mutex.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Limiter is a token bucket rate limiter.
//
// The bucket holds at most Burst tokens and refills at rate tokens
// per second. When Allow is called, one token is removed if
// available; otherwise the call returns false immediately and does
// not block the caller.
//
// All methods are safe for concurrent use.
type Limiter struct {
	clock Clock
	rate  float64       // tokens added per second
	burst float64       // bucket capacity

	mu         sync.Mutex
	tokens     float64   // current number of tokens
	lastUpdate time.Time // last time tokens were computed
}

// NewLimiter returns a Limiter that refills at rate tokens per
// second up to a maximum of burst tokens.
//
// rate must be > 0 and burst must be >= 1; otherwise NewLimiter
// panics. A zero or negative rate has no useful meaning for a
// token bucket, and a burst of zero would never allow any call to
// succeed.
func NewLimiter(rate float64, burst int, clock Clock) *Limiter {
	if rate <= 0 {
		panic("go_sdk: rate must be greater than zero")
	}
	if burst < 1 {
		panic("go_sdk: burst must be at least 1")
	}
	if clock == nil {
		panic("go_sdk: clock must not be nil")
	}
	return &Limiter{
		clock:      clock,
		rate:       rate,
		burst:      float64(burst),
		tokens:     float64(burst), // start full so the first burst of calls succeeds
		lastUpdate: clock.Now(),
	}
}

// refill recomputes the token count based on elapsed time since
// the last update. It must be called with the mutex held.
//
// Negative time deltas (which can happen only if a fake clock is
// rewound) are treated as zero: tokens are never removed by the
// passage of time.
func (l *Limiter) refill(now time.Time) {
	elapsed := now.Sub(l.lastUpdate)
	if elapsed < 0 {
		elapsed = 0
	}
	add := float64(elapsed) / float64(time.Second) * l.rate
	l.tokens += add
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
	l.lastUpdate = now
}

// Allow reports whether one operation may proceed at this moment.
// It returns true if a token was available (and removes one) and
// false otherwise. The call never blocks.
//
// Allow is deterministic when given a deterministic Clock: a fake
// clock that does not advance between two calls will make the
// second call see the state left by the first.
func (l *Limiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN reports whether n operations may proceed at this moment.
// It returns true if at least n tokens were available (and removes
// n) and false otherwise. If n exceeds Burst, AllowN always
// returns false because the bucket can never hold that many.
//
// A false result leaves the token count unchanged.
//
// n must be >= 0; n == 0 always succeeds and consumes nothing.
func (l *Limiter) AllowN(n int) bool {
	if n < 0 {
		panic("go_sdk: n must be non-negative")
	}
	if n == 0 {
		return true
	}
	if float64(n) > l.burst {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill(l.clock.Now())
	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return true
	}
	return false
}

// Tokens returns the current number of available tokens, rounded
// down to an integer. It triggers a refill so the returned value
// reflects the passage of time up to now. Useful for inspection,
// not for coordination.
func (l *Limiter) Tokens() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill(l.clock.Now())
	return int(l.tokens)
}

// Rate returns the configured refill rate in tokens per second.
// It is constant for the lifetime of the Limiter.
func (l *Limiter) Rate() float64 { return l.rate }

// Burst returns the configured bucket capacity in whole tokens.
// It is constant for the lifetime of the Limiter.
func (l *Limiter) Burst() int { return int(l.burst) }
