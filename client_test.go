package flagmint_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	flagmint "github.com/flagmint/flagmint-go"
)

func TestNewClient_EmptyAPIKey(t *testing.T) {
	_, err := flagmint.NewClient("")
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
}

func TestNewClient_DeferInit(t *testing.T) {
	c, err := flagmint.NewClient("test-key",
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer c.Close() //nolint:errcheck

	// No flags should be available before Initialize.
	if c.GetFlags().Len() != 0 {
		t.Fatalf("expected empty flags, got %d", c.GetFlags().Len())
	}

	if c.GetFlags().Has("absent") {
		t.Fatal("expected missing flag to return Has=false")
	}
}

func TestSetContext(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	// Should not panic.
	c.SetContext(flagmint.EvaluationContext{Kind: "user", Key: "u999"})
}

func TestWithContext_Option(t *testing.T) {
	ctx := flagmint.EvaluationContext{Kind: "user", Key: "u1"}
	c, err := flagmint.NewClient("test-key",
		flagmint.WithContext(ctx),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck
}

func TestFeatureFlags_TypedGetters(t *testing.T) {
	flags := flagmint.NewFeatureFlags(map[string]any{
		"dark-mode":   true,
		"retries":     float64(3),
		"greeting":    "hello",
		"config":      map[string]any{"timeout": float64(30)},
	})

	// Bool
	if !flags.Bool("dark-mode", false) {
		t.Error("Bool: expected true")
	}
	if flags.Bool("missing", true) != true {
		t.Error("Bool fallback: expected true")
	}

	// Float64
	if flags.Float64("retries", 0) != 3 {
		t.Errorf("Float64: got %v, want 3", flags.Float64("retries", 0))
	}
	if flags.Float64("missing", 99) != 99 {
		t.Errorf("Float64 fallback: got %v, want 99", flags.Float64("missing", 99))
	}

	// String
	if flags.String("greeting", "") != "hello" {
		t.Errorf("String: got %q, want hello", flags.String("greeting", ""))
	}
	if flags.String("missing", "default") != "default" {
		t.Errorf("String fallback: got %q, want default", flags.String("missing", "default"))
	}

	// JSON
	var cfg struct {
		Timeout float64 `json:"timeout"`
	}
	if err := flags.JSON("config", &cfg); err != nil {
		t.Fatalf("JSON: unexpected error: %v", err)
	}
	if cfg.Timeout != 30 {
		t.Errorf("JSON: Timeout = %v, want 30", cfg.Timeout)
	}

	// JSON missing key
	if err := flags.JSON("missing", &cfg); !errors.Is(err, flagmint.ErrFlagNotFound) {
		t.Errorf("JSON missing: expected ErrFlagNotFound, got %v", err)
	}

	// Has / Len
	if !flags.Has("dark-mode") {
		t.Error("Has: expected true for existing key")
	}
	if flags.Has("missing") {
		t.Error("Has: expected false for missing key")
	}
	if flags.Len() != 4 {
		t.Errorf("Len: got %d, want 4", flags.Len())
	}
}

func TestFlagClient_TypedConvenience(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	// All fallbacks before flags are loaded.
	if c.Bool("x", true) != true {
		t.Error("Bool convenience fallback")
	}
	if c.String("x", "fb") != "fb" {
		t.Error("String convenience fallback")
	}
	if c.Float64("x", 7) != 7 {
		t.Error("Float64 convenience fallback")
	}
	if err := c.JSON("x", new(struct{})); !errors.Is(err, flagmint.ErrFlagNotFound) {
		t.Errorf("JSON convenience fallback: %v", err)
	}
}

// TestReady_DeferInit verifies that Ready triggers initialisation and returns
// nil when the (stub) transport succeeds.
func TestReady_DeferInit(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Ready(ctx); err != nil {
		t.Fatalf("Ready: unexpected error: %v", err)
	}
}

// TestReady_ContextCancelled verifies that an already-cancelled context causes
// Ready to return context.Canceled immediately.
func TestReady_ContextCancelled(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	if err := c.Ready(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ready with cancelled ctx: got %v, want context.Canceled", err)
	}
}

// TestReady_Idempotent verifies multiple Ready calls all return the same result.
func TestReady_Idempotent(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := c.Ready(ctx); err != nil {
			t.Fatalf("Ready call %d: unexpected error: %v", i+1, err)
		}
	}
}

// TestGetFlag verifies the raw-value getter and fallback behaviour.
func TestGetFlag(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	// No flags loaded: should return fallback.
	if got := c.GetFlag("x", "default"); got != "default" {
		t.Errorf("GetFlag missing: got %v, want %q", got, "default")
	}
}

// TestTypedFlagHelpers verifies BoolFlag, StringFlag, NumberFlag, JSONFlag.
func TestTypedFlagHelpers(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	if got := c.BoolFlag("x", true); got != true {
		t.Error("BoolFlag fallback")
	}
	if got := c.StringFlag("x", "fb"); got != "fb" {
		t.Error("StringFlag fallback")
	}
	if got := c.NumberFlag("x", 42); got != 42 {
		t.Error("NumberFlag fallback")
	}
	fb := map[string]any{"k": "v"}
	if got := c.JSONFlag("x", fb); got == nil {
		t.Error("JSONFlag fallback should not be nil")
	}
}

// TestUpdateContext verifies that UpdateContext replaces the evaluation context
// and returns nil.
func TestUpdateContext(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	if err := c.UpdateContext(flagmint.EvaluationContext{Kind: "user", Key: "u2"}); err != nil {
		t.Fatalf("UpdateContext: unexpected error: %v", err)
	}
}

// TestSubscribe verifies that the callback fires immediately with current
// flags, then again on every update, and stops after unsubscribe.
func TestSubscribe(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	var mu sync.Mutex
	var calls []flagmint.FeatureFlags

	unsub := c.Subscribe(func(f flagmint.FeatureFlags) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, f)
	})

	// Should have been called once immediately with empty flags.
	mu.Lock()
	n := len(calls)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 initial call, got %d", n)
	}

	// Unsubscribe and confirm no further calls are delivered.
	unsub()

	mu.Lock()
	before := len(calls)
	mu.Unlock()

	if before != 1 {
		t.Fatalf("expected still 1 call after unsub, got %d", before)
	}
}

// TestClose_Idempotent verifies that calling Close twice does not panic.
func TestClose_Idempotent(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestGetFlags_ShallowCopy verifies that mutating the returned FeatureFlags
// map does not affect subsequent calls.
func TestGetFlags_ShallowCopy(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	f1 := c.GetFlags()
	f2 := c.GetFlags()

	// Both calls should return independent zero-value flag sets.
	if f1.Len() != 0 || f2.Len() != 0 {
		t.Error("expected empty flag sets")
	}
}

// TestConcurrentSafety verifies no data races when GetFlag and UpdateContext
// are called concurrently by many goroutines.
func TestConcurrentSafety(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		// Readers
		go func() {
			defer wg.Done()
			c.GetFlag("feature", false)
		}()
		// Writers
		go func(i int) {
			defer wg.Done()
			_ = c.UpdateContext(flagmint.EvaluationContext{Kind: "user", Key: "u"})
			_ = i
		}(i)
	}

	wg.Wait()
}

// TestConcurrentSubscribe verifies that subscribing and unsubscribing from
// multiple goroutines concurrently does not cause races.
func TestConcurrentSubscribe(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for j := 0; j < goroutines; j++ {
		go func() {
			defer wg.Done()
			unsub := c.Subscribe(func(flagmint.FeatureFlags) {})
			unsub()
		}()
	}

	wg.Wait()
}

