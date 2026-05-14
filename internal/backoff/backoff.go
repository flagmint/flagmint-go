// Package backoff provides exponential back-off with jitter.
package backoff

import (
	"math"
	"math/rand"
	"time"
)

// Backoff computes exponential back-off delays with optional jitter.
// The zero value is not useful; use a struct literal with explicit fields.
//
//	b := &backoff.Backoff{
//	    Base:       time.Second,
//	    Multiplier: 2,
//	    MaxDelay:   60 * time.Second,
//	    Jitter:     0.2,
//	}
type Backoff struct {
	// Base is the initial delay returned on the first call to Next.
	Base time.Duration
	// Multiplier is applied on each successive attempt (e.g. 2.0 doubles).
	Multiplier float64
	// MaxDelay caps the delay before jitter is applied.
	MaxDelay time.Duration
	// Jitter is the fraction of the delay applied as a symmetric random offset,
	// e.g. 0.2 means ±20%.
	Jitter  float64
	attempt int
}

// Next returns the next back-off duration and advances the internal counter.
// The sequence is: Base, Base*Multiplier, Base*Multiplier², … capped at MaxDelay.
// A random jitter of ±Jitter*delay is added to spread concurrent retries.
func (b *Backoff) Next() time.Duration {
	delay := float64(b.Base) * math.Pow(b.Multiplier, float64(b.attempt))
	if delay > float64(b.MaxDelay) {
		delay = float64(b.MaxDelay)
	}
	if b.Jitter > 0 {
		// Apply symmetric jitter: delay × Jitter × uniform(-1, 1).
		// Go 1.20+ automatically seeds the global rand source, so no explicit
		// seeding is required.
		jitter := delay * b.Jitter * (2*rand.Float64() - 1)
		delay += jitter
	}
	if delay < 0 {
		delay = 0
	}
	b.attempt++
	return time.Duration(delay)
}

// Reset resets the attempt counter so the next call to Next returns the base delay.
func (b *Backoff) Reset() { b.attempt = 0 }
