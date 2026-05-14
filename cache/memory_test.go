package cache_test

import (
	"sync"
	"testing"

	"github.com/flagmint/flagmint-go/cache"
)

func TestMemoryCache(t *testing.T) {
	c := cache.NewMemoryCache()

	// Miss before insertion.
	if _, ok := c.Get("k1"); ok {
		t.Fatal("expected cache miss")
	}

	// Hit after insertion.
	c.Set("k1", "value1")
	v, ok := c.Get("k1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if v != "value1" {
		t.Errorf("got %v, want value1", v)
	}

	// Delete removes the entry.
	c.Delete("k1")
	if _, ok := c.Get("k1"); ok {
		t.Fatal("expected cache miss after delete")
	}

	// Flush removes all entries.
	c.Set("a", 1)
	c.Set("b", 2)
	c.Flush()
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected cache miss after flush")
	}
}

// TestMemoryCache_MultipleKeys verifies storing and retrieving multiple keys.
func TestMemoryCache_MultipleKeys(t *testing.T) {
	c := cache.NewMemoryCache()

	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Set("key3", "value3")

	v1, ok1 := c.Get("key1")
	v2, ok2 := c.Get("key2")
	v3, ok3 := c.Get("key3")

	if !ok1 || v1 != "value1" {
		t.Error("key1 retrieval failed")
	}
	if !ok2 || v2 != "value2" {
		t.Error("key2 retrieval failed")
	}
	if !ok3 || v3 != "value3" {
		t.Error("key3 retrieval failed")
	}
}

// TestMemoryCache_Overwrite verifies that Set overwrites existing values.
func TestMemoryCache_Overwrite(t *testing.T) {
	c := cache.NewMemoryCache()

	c.Set("key", "value1")
	if v, _ := c.Get("key"); v != "value1" {
		t.Error("initial value incorrect")
	}

	c.Set("key", "value2")
	if v, _ := c.Get("key"); v != "value2" {
		t.Error("overwritten value incorrect")
	}
}

// TestMemoryCache_DeleteNonExistent verifies deleting a non-existent key is safe.
func TestMemoryCache_DeleteNonExistent(t *testing.T) {
	c := cache.NewMemoryCache()

	// Should not panic
	c.Delete("non-existent")

	// Verify nothing was added
	if _, ok := c.Get("non-existent"); ok {
		t.Error("key should still not exist")
	}
}

// TestMemoryCache_FlushEmpty verifies flushing an empty cache is safe.
func TestMemoryCache_FlushEmpty(t *testing.T) {
	c := cache.NewMemoryCache()

	// Should not panic
	c.Flush()

	// Verify cache is still usable
	c.Set("key", "value")
	if v, ok := c.Get("key"); !ok || v != "value" {
		t.Error("cache not usable after flush empty")
	}
}

// TestMemoryCache_ConcurrentAccess verifies thread safety with concurrent operations.
func TestMemoryCache_ConcurrentAccess(t *testing.T) {
	c := cache.NewMemoryCache()

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := "key"
				val := "value"
				c.Set(key, val)
				if v, ok := c.Get(key); !ok || v != val {
					t.Errorf("concurrent access failed for goroutine %d", id)
				}
			}
		}(g)
	}

	wg.Wait()
}

// TestMemoryCache_Various types verifies storing various value types.
func TestMemoryCache_VariousTypes(t *testing.T) {
	c := cache.NewMemoryCache()

	// String
	c.Set("string", "hello")
	if v, _ := c.Get("string"); v != "hello" {
		t.Error("string value mismatch")
	}

	// Number
	c.Set("number", float64(42))
	if v, _ := c.Get("number"); v != float64(42) {
		t.Error("number value mismatch")
	}

	// Boolean
	c.Set("bool", true)
	if v, _ := c.Get("bool"); v != true {
		t.Error("bool value mismatch")
	}

	// Map
	m := map[string]any{"key": "value"}
	c.Set("map", m)
	if v, _ := c.Get("map"); v == nil {
		t.Error("map value should not be nil")
	}

	// Nil (edge case)
	c.Set("nil", nil)
	if v, ok := c.Get("nil"); !ok || v != nil {
		t.Error("nil value mismatch")
	}
}
