package cache_test

import (
	"sync"
	"testing"
	"time"

	flagmint "github.com/flagmint/flagmint-go"
	"github.com/flagmint/flagmint-go/cache"
)

// sampleFlags builds a FeatureFlags value for use in tests.
func sampleFlags(entries map[string]any) flagmint.FeatureFlags {
	return flagmint.NewFeatureFlags(entries)
}

// sampleContext returns an EvaluationContext for use in tests.
func sampleContext(key string) *flagmint.EvaluationContext {
	return &flagmint.EvaluationContext{Kind: "user", Key: key}
}

// --- MemoryCache tests ---

// TestMemoryCache_RoundTrip verifies that SaveFlags → LoadFlags returns the same data.
func TestMemoryCache_RoundTrip(t *testing.T) {
	c := cache.NewMemoryCache(cache.DefaultTTL)
	flags := sampleFlags(map[string]any{"dark-mode": true, "retries": float64(3)})

	if err := c.SaveFlags("api-key", flags); err != nil {
		t.Fatalf("SaveFlags: unexpected error: %v", err)
	}

	loaded, err := c.LoadFlags("api-key")
	if err != nil {
		t.Fatalf("LoadFlags: unexpected error: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadFlags: expected non-nil flags, got nil")
	}
	if !loaded.Bool("dark-mode", false) {
		t.Error("LoadFlags: dark-mode flag should be true")
	}
	if loaded.Float64("retries", 0) != 3 {
		t.Errorf("LoadFlags: retries = %v, want 3", loaded.Float64("retries", 0))
	}
}

// TestMemoryCache_MissingKey verifies that LoadFlags returns nil for an unknown key.
func TestMemoryCache_MissingKey(t *testing.T) {
	c := cache.NewMemoryCache(cache.DefaultTTL)

	loaded, err := c.LoadFlags("unknown-key")
	if err != nil {
		t.Fatalf("LoadFlags: unexpected error: %v", err)
	}
	if loaded != nil {
		t.Errorf("LoadFlags: expected nil for unknown key, got %v", loaded)
	}
}

// TestMemoryCache_TTLExpiry verifies that entries return nil after TTL elapses.
func TestMemoryCache_TTLExpiry(t *testing.T) {
	shortTTL := 50 * time.Millisecond
	c := cache.NewMemoryCache(shortTTL)
	flags := sampleFlags(map[string]any{"feature": true})

	if err := c.SaveFlags("api-key", flags); err != nil {
		t.Fatalf("SaveFlags: %v", err)
	}

	// Entry should be valid before TTL elapses.
	if loaded, err := c.LoadFlags("api-key"); err != nil || loaded == nil {
		t.Fatalf("LoadFlags before expiry: err=%v, loaded=%v", err, loaded)
	}

	// Wait for the TTL to elapse.
	time.Sleep(shortTTL + 10*time.Millisecond)

	// Entry should now be expired (nil, nil).
	expired, err := c.LoadFlags("api-key")
	if err != nil {
		t.Fatalf("LoadFlags after expiry: unexpected error: %v", err)
	}
	if expired != nil {
		t.Error("LoadFlags after expiry: expected nil, got non-nil flags")
	}
}

// TestMemoryCache_TTLRefresh verifies that a second SaveFlags resets the TTL timer.
func TestMemoryCache_TTLRefresh(t *testing.T) {
	halfTTL := 40 * time.Millisecond
	fullTTL := halfTTL * 2
	c := cache.NewMemoryCache(fullTTL)
	flags := sampleFlags(map[string]any{"feature": true})

	// First save.
	if err := c.SaveFlags("api-key", flags); err != nil {
		t.Fatalf("first SaveFlags: %v", err)
	}

	// Wait less than the full TTL, then save again to reset the timer.
	time.Sleep(halfTTL)
	if err := c.SaveFlags("api-key", flags); err != nil {
		t.Fatalf("second SaveFlags: %v", err)
	}

	// Wait less than a full TTL from the second save — entry should still be valid.
	time.Sleep(halfTTL)
	refreshed, err := c.LoadFlags("api-key")
	if err != nil {
		t.Fatalf("LoadFlags after refresh: %v", err)
	}
	if refreshed == nil {
		t.Error("LoadFlags after refresh: expected non-nil flags, got nil")
	}
}

// TestMemoryCache_ContextRoundTrip verifies that SaveContext → LoadContext returns
// the same data.
func TestMemoryCache_ContextRoundTrip(t *testing.T) {
	c := cache.NewMemoryCache(cache.DefaultTTL)
	ctx := sampleContext("user-123")

	if err := c.SaveContext("api-key", ctx); err != nil {
		t.Fatalf("SaveContext: %v", err)
	}

	loaded, err := c.LoadContext("api-key")
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadContext: expected non-nil context, got nil")
	}
	if loaded.Key != "user-123" {
		t.Errorf("LoadContext: key = %q, want user-123", loaded.Key)
	}
}

// TestMemoryCache_ContextMissingKey verifies that LoadContext returns nil for an
// unknown key.
func TestMemoryCache_ContextMissingKey(t *testing.T) {
	c := cache.NewMemoryCache(cache.DefaultTTL)

	loaded, err := c.LoadContext("unknown-key")
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	if loaded != nil {
		t.Errorf("LoadContext: expected nil for unknown key, got %v", loaded)
	}
}

// TestMemoryCache_SaveFlagsOverwrites verifies that a second SaveFlags replaces
// the previous entry.
func TestMemoryCache_SaveFlagsOverwrites(t *testing.T) {
	c := cache.NewMemoryCache(cache.DefaultTTL)
	firstFlags := sampleFlags(map[string]any{"version": float64(1)})
	secondFlags := sampleFlags(map[string]any{"version": float64(2)})

	_ = c.SaveFlags("api-key", firstFlags)
	_ = c.SaveFlags("api-key", secondFlags)

	loaded, err := c.LoadFlags("api-key")
	if err != nil {
		t.Fatalf("LoadFlags: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadFlags: expected non-nil flags")
	}
	if loaded.Float64("version", 0) != 2 {
		t.Errorf("LoadFlags: version = %v, want 2", loaded.Float64("version", 0))
	}
}

// TestMemoryCache_ConcurrentAccess verifies goroutine safety under concurrent
// reads and writes.
func TestMemoryCache_ConcurrentAccess(t *testing.T) {
	c := cache.NewMemoryCache(cache.DefaultTTL)
	flags := sampleFlags(map[string]any{"feature": true})
	ctx := sampleContext("user-concurrent")

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers
	for workerID := 0; workerID < goroutines; workerID++ {
		go func() {
			defer wg.Done()
			_ = c.SaveFlags("api-key", flags)
			_ = c.SaveContext("api-key", ctx)
		}()
	}

	// Readers
	for workerID := 0; workerID < goroutines; workerID++ {
		go func() {
			defer wg.Done()
			_, _ = c.LoadFlags("api-key")
			_, _ = c.LoadContext("api-key")
		}()
	}

	wg.Wait()
}

// --- NopCache tests ---

// TestNopCache_AllMethodsReturnNil verifies that NopCache discards writes and
// returns nil on all reads without error.
func TestNopCache_AllMethodsReturnNil(t *testing.T) {
	var nop cache.NopCache
	flags := sampleFlags(map[string]any{"feature": true})
	ctx := sampleContext("user-nop")

	// SaveFlags and SaveContext should succeed silently.
	if err := nop.SaveFlags("api-key", flags); err != nil {
		t.Errorf("NopCache.SaveFlags: unexpected error: %v", err)
	}
	if err := nop.SaveContext("api-key", ctx); err != nil {
		t.Errorf("NopCache.SaveContext: unexpected error: %v", err)
	}

	// LoadFlags should always return nil, nil.
	loadedFlags, err := nop.LoadFlags("api-key")
	if err != nil {
		t.Errorf("NopCache.LoadFlags: unexpected error: %v", err)
	}
	if loadedFlags != nil {
		t.Errorf("NopCache.LoadFlags: expected nil, got %v", loadedFlags)
	}

	// LoadContext should always return nil, nil.
	loadedCtx, err := nop.LoadContext("api-key")
	if err != nil {
		t.Errorf("NopCache.LoadContext: unexpected error: %v", err)
	}
	if loadedCtx != nil {
		t.Errorf("NopCache.LoadContext: expected nil, got %v", loadedCtx)
	}
}
