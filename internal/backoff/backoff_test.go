package backoff_test

import (
	"testing"
	"time"

	"github.com/flagmint/flagmint-go/internal/backoff"
)

func TestBackoff_Next_Sequence(t *testing.T) {
	b := &backoff.Backoff{
		Base:       time.Second,
		Multiplier: 2.0,
		MaxDelay:   60 * time.Second,
		Jitter:     0, // no jitter for deterministic test
	}

	cases := []time.Duration{
		1 * time.Second,  // attempt 0: 1s * 2^0
		2 * time.Second,  // attempt 1: 1s * 2^1
		4 * time.Second,  // attempt 2: 1s * 2^2
		8 * time.Second,  // attempt 3: 1s * 2^3
		16 * time.Second, // attempt 4: 1s * 2^4
		32 * time.Second, // attempt 5: 1s * 2^5
		60 * time.Second, // attempt 6: capped at 60s
		60 * time.Second, // attempt 7: still capped
	}

	for i, want := range cases {
		got := b.Next()
		if got != want {
			t.Errorf("attempt %d: got %v, want %v", i, got, want)
		}
	}
}

func TestBackoff_Reset(t *testing.T) {
	b := &backoff.Backoff{
		Base:       500 * time.Millisecond,
		Multiplier: 2.0,
		MaxDelay:   60 * time.Second,
		Jitter:     0,
	}

	// Advance a few steps.
	b.Next()
	b.Next()
	b.Next()

	b.Reset()
	got := b.Next()
	if got != 500*time.Millisecond {
		t.Errorf("after Reset, Next() = %v, want 500ms", got)
	}
}

func TestBackoff_Cap(t *testing.T) {
	b := &backoff.Backoff{
		Base:       time.Second,
		Multiplier: 10.0,
		MaxDelay:   5 * time.Second,
		Jitter:     0,
	}

	// After a few steps we should be capped.
	for i := 0; i < 20; i++ {
		got := b.Next()
		if got > 5*time.Second {
			t.Errorf("step %d: delay %v exceeds cap %v", i, got, 5*time.Second)
		}
	}
}

func TestBackoff_Jitter(t *testing.T) {
	b := &backoff.Backoff{
		Base:       time.Second,
		Multiplier: 1.0, // constant base
		MaxDelay:   60 * time.Second,
		Jitter:     0.2,
	}

	// With 20% jitter around 1s the delay must be in [0.8s, 1.2s].
	for i := 0; i < 100; i++ {
		b.Reset()
		got := b.Next()
		if got < 800*time.Millisecond || got > 1200*time.Millisecond {
			t.Errorf("jitter out of range: got %v", got)
		}
	}
}

func TestBackoff_NonNegative(t *testing.T) {
	// Even with extreme jitter the returned delay must be ≥ 0.
	b := &backoff.Backoff{
		Base:       time.Nanosecond,
		Multiplier: 1.0,
		MaxDelay:   time.Second,
		Jitter:     0.99,
	}

	for i := 0; i < 50; i++ {
		if d := b.Next(); d < 0 {
			t.Errorf("negative delay: %v", d)
		}
	}
}
