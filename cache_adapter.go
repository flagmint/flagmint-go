package flagmint

import (
	"sync"
	"time"
)

// DefaultCacheTTL is the default time-to-live for cached flags.
// Matches the JS SDK's DEFAULT_CACHE_TTL (24 * 60 * 60 * 1000 ms).
const DefaultCacheTTL = 24 * time.Hour

// CacheAdapter provides persistence for flags and evaluation context
// across client restarts. Implementations must be safe for concurrent use.
type CacheAdapter interface {
	// LoadFlags returns cached flags for the given API key, or nil if
	// no valid (non-expired) cache entry exists. Errors indicate storage
	// failure, not cache misses.
	LoadFlags(apiKey string) (*FeatureFlags, error)

	// SaveFlags persists flags for the given API key. Called on every
	// flag update — implementations should be fast (async write is fine).
	SaveFlags(apiKey string, flags FeatureFlags) error

	// LoadContext returns the cached evaluation context, or nil if none exists.
	// Context entries have no TTL — they persist until overwritten.
	LoadContext(apiKey string) (*EvaluationContext, error)

	// SaveContext persists the evaluation context. Unlike flags, the context
	// has no TTL and remains valid until explicitly overwritten.
	SaveContext(apiKey string, ctx *EvaluationContext) error
}

// flagCacheEntry holds a FeatureFlags value and its storage timestamp for TTL checks.
type flagCacheEntry struct {
	flags    FeatureFlags
	storedAt time.Time
}

// internalMemoryCache is the default in-memory CacheAdapter used when no custom
// adapter is provided. It is goroutine-safe and uses DefaultCacheTTL for flags.
// Context entries have no TTL — they persist until overwritten.
type internalMemoryCache struct {
	mu       sync.RWMutex
	ttl      time.Duration
	flags    map[string]flagCacheEntry
	contexts map[string]*EvaluationContext
}

func newDefaultMemoryCache() *internalMemoryCache {
	return &internalMemoryCache{
		ttl:      DefaultCacheTTL,
		flags:    make(map[string]flagCacheEntry),
		contexts: make(map[string]*EvaluationContext),
	}
}

// LoadFlags returns the cached flags for apiKey, or nil if the entry is absent
// or has expired.
func (c *internalMemoryCache) LoadFlags(apiKey string) (*FeatureFlags, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.flags[apiKey]
	if !ok {
		return nil, nil
	}
	if c.ttl > 0 && time.Since(entry.storedAt) > c.ttl {
		return nil, nil
	}
	flags := entry.flags
	return &flags, nil
}

// SaveFlags stores flags for apiKey, resetting the TTL timer.
func (c *internalMemoryCache) SaveFlags(apiKey string, flags FeatureFlags) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flags[apiKey] = flagCacheEntry{flags: flags, storedAt: time.Now()}
	return nil
}

// LoadContext returns the cached evaluation context for apiKey, or nil if none exists.
func (c *internalMemoryCache) LoadContext(apiKey string) (*EvaluationContext, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ctx, ok := c.contexts[apiKey]
	if !ok {
		return nil, nil
	}
	return ctx, nil
}

// SaveContext persists the evaluation context for apiKey.
func (c *internalMemoryCache) SaveContext(apiKey string, ctx *EvaluationContext) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contexts[apiKey] = ctx
	return nil
}
