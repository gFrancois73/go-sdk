# go-sdk

A token bucket rate limiter that controls how many operations per second are allowed, with burst support. The bucket refills continuously at a fixed rate and caps at a burst size, so short bursts of traffic are allowed while the long-term rate is bounded.

## Usage

```go
package main

import (
	"fmt"
	"time"

	go_sdk "go_sdk"
)

func main() {
	lim := go_sdk.NewLimiter(10, 5, go_sdk.NewSystemClock())

	// First 5 calls succeed (burst), then they start failing
	// until time refills the bucket.
	for i := 0; i < 7; i++ {
		fmt.Printf("call %d: allowed=%v\n", i, lim.Allow())
		time.Sleep(50 * time.Millisecond)
	}
}
```

Exported names:

- `NewLimiter(rate float64, burst int, clock Clock) *Limiter` constructs a limiter. `rate` is tokens per second and must be > 0; `burst` is the bucket capacity and must be >= 1.
- `NewSystemClock() Clock` returns a clock backed by `time.Now`.
- `Clock` is the interface any clock must implement: `Now() time.Time`.
- `SystemClock` is the concrete system-clock type.
- `Limiter` is the rate limiter. Methods:
  - `Allow() bool` — removes one token if available.
  - `AllowN(n int) bool` — removes `n` tokens if available; returns false if `n` exceeds burst or there are not enough tokens. A false result consumes nothing.
  - `Tokens() int` — returns the current available tokens (floored), after refilling.
  - `Rate() float64` — returns the configured rate.
  - `Burst() int` — returns the configured burst.

## Why this exists

This library exists for code that needs a simple, dependency-free way to bound the rate of outbound operations — API calls, log writes, background tasks — without pulling in a framework. The trade-off is simplicity over flexibility: there is no blocking `Wait`, no reservation API, and no per-key bucketing. If you need those, a larger library is the right choice.

## The awkward edge

`AllowN` returns false without consuming any tokens when there are not enough. This is intentional: a failed batch request should not silently drain the bucket and starve later callers. If `n` is larger than `burst`, the call always fails immediately regardless of how full the bucket is, because the bucket can never hold that many tokens.

The bucket starts full so the first burst of calls succeeds immediately after construction. If you need a cold start, drain it explicitly with `AllowN(burst)`.
