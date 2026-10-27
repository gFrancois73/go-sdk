package go_sdk

import (
	"math"
	"testing"
	"time"
)

// newFakeClock creates a fakeClock anchored at a fixed moment so
// that tests never touch wall-clock time. Any advance is explicit.
func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1_000_000, 0)}
}

// assertAllow compares the boolean result of Allow against an
// expected value, using the testing package for failure reporting.
func assertAllowed(t *testing.T, lim *Limiter, want bool) {
	t.Helper()
	got := lim.Allow()
	if got != want {
		t.Errorf("Allow() = %v, want %v", got, want)
	}
}

func TestAllowWithinBurst(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(10, 3, clk)
	// Bucket starts full; three calls should drain it.
	assertAllowed(t, lim, true)
	assertAllowed(t, lim, true)
	assertAllowed(t, lim, true)
	assertAllowed(t, lim, false)
}

func TestRefillAfterTime(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(10, 3, clk)
	// Drain the bucket completely.
	for i := 0; i < 3; i++ {
		if !lim.Allow() {
			t.Fatalf("expected initial call %d to be allowed", i)
		}
	}
	// 100 ms at 10/s adds exactly 1 token.
	clk.Advance(100 * time.Millisecond)
	assertAllowed(t, lim, true)
	assertAllowed(t, lim, false)
}

func TestRefillCapsAtBurst(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(10, 3, clk)
	// Drain so we can observe refill behavior.
	for i := 0; i < 3; i++ {
		lim.Allow()
	}
	// Wait long enough to overflow the bucket.
	clk.Advance(1 * time.Hour)
	// Only 3 tokens should be available.
	assertAllowed(t, lim, true)
	assertAllowed(t, lim, true)
	assertAllowed(t, lim, true)
	assertAllowed(t, lim, false)
}

func TestNegativeTimeDeltaTreatedAsZero(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(10, 3, clk)
	// Consume one so tokens < burst.
	lim.Allow()
	// Rewinding the clock must not grant extra tokens and must not
	// panic or corrupt internal state.
	clk.Advance(-1 * time.Hour)
	before := lim.Tokens()
	lim.Allow()
	after := lim.Tokens()
	// one token should have been consumed, no more.
	if math.Abs(float64(before-after)-1.0) > 1e-9 {
		t.Errorf("expected 1 token consumed, got before=%d after=%d", before, after)
	}
}

func TestAllowNWithPartialAvailability(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(10, 5, clk)
	// Consume 3, leaving 2.
	if !lim.AllowN(3) {
		t.Fatalf("AllowN(3) should succeed on a full bucket")
	}
	// 2 available: AllowN(2) succeeds, AllowN(3) fails and must
	// not change the token count.
	if !lim.AllowN(2) {
		t.Fatalf("AllowN(2) should succeed with 2 tokens")
	}
	if lim.AllowN(3) {
		t.Fatalf("AllowN(3) should fail with 0 tokens")
	}
	// Still 0 tokens.
	if lim.Allow() {
		t.Fatalf("Allow should fail after failed AllowN drained nothing")
	}
}

func TestAllowNLargerThanBurstFails(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(10, 3, clk)
	if lim.AllowN(4) {
		t.Fatalf("AllowN(4) should fail when burst is 3")
	}
	// Must not have consumed anything.
	if lim.Tokens() != 3 {
		t.Errorf("expected 3 tokens after failed AllowN, got %d", lim.Tokens())
	}
}

func TestAllowNZeroAlwaysSucceeds(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(10, 1, clk)
	if !lim.AllowN(0) {
		t.Fatalf("AllowN(0) should always succeed")
	}
}

func TestPanicOnInvalidRate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for zero rate")
		}
	}()
	NewLimiter(0, 1, NewSystemClock())
}

func TestPanicOnInvalidBurst(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for zero burst")
		}
	}()
	NewLimiter(1, 0, NewSystemClock())
}

func TestPanicOnNilClock(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for nil clock")
		}
	}()
	NewLimiter(1, 1, nil)
}

func TestRateAndBurstAccessors(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(7.5, 4, clk)
	if lim.Rate() != 7.5 {
		t.Errorf("Rate() = %v, want 7.5", lim.Rate())
	}
	if lim.Burst() != 4 {
		t.Errorf("Burst() = %d, want 4", lim.Burst())
	}
}

func TestTokensReflectsRefill(t *testing.T) {
	clk := newFakeClock()
	lim := NewLimiter(10, 5, clk)
	for i := 0; i < 5; i++ {
		lim.Allow()
	}
	if lim.Tokens() != 0 {
		t.Fatalf("expected 0 tokens, got %d", lim.Tokens())
	}
	clk.Advance(250 * time.Millisecond)
	// 250ms at 10/s = 2.5 tokens; floored to 2.
	if lim.Tokens() != 2 {
		t.Errorf("expected 2 tokens, got %d", lim.Tokens())
	}
}
