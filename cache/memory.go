package cache

import (
	"sync"
	"time"

	flagmint "github.com/flagmint/flagmint-go"
)

// flagEntry holds a FeatureFlags value and its storage timestamp for TTL checks.
type flagEntry struct {
	flags    flagmint.FeatureFlags
	storedAt time.Time
}

// MemoryCache is a simple in-memory CacheAdapter with TTL.
// Safe for concurrent use.
type MemoryCache struct {
	mu       sync.RWMutex
	ttl      time.Duration
	flags    map[string]flagEntry
	contexts map[string]*flagmint.EvaluationContext
}

// NewMemoryCache returns a new MemoryCache with the given TTL.
// Use DefaultTTL for the standard 24-hour TTL.
// A TTL of zero disables flag expiry.
func NewMemoryCache(ttl time.Duration) *MemoryCache {
	return &MemoryCache{
		ttl:      ttl,
		flags:    make(map[string]flagEntry),
		contexts: make(map[string]*flagmint.EvaluationContext),
	}
}

// LoadFlags returns the cached flags for apiKey, or nil if the entry is absent
// or has expired.
func (m *MemoryCache) LoadFlags(apiKey string) (*flagmint.FeatureFlags, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.flags[apiKey]
	if !ok {
		return nil, nil
	}
	if m.ttl > 0 && time.Since(entry.storedAt) > m.ttl {
		return nil, nil
	}
	flags := entry.flags
	return &flags, nil
}

// SaveFlags stores flags for apiKey, resetting the TTL timer.
func (m *MemoryCache) SaveFlags(apiKey string, flags flagmint.FeatureFlags) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flags[apiKey] = flagEntry{flags: flags, storedAt: time.Now()}
	return nil
}

// LoadContext returns the cached evaluation context for apiKey, or nil if none exists.
func (m *MemoryCache) LoadContext(apiKey string) (*flagmint.EvaluationContext, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ctx, ok := m.contexts[apiKey]
	if !ok {
		return nil, nil
	}
	return ctx, nil
}

// SaveContext persists the evaluation context for apiKey.
func (m *MemoryCache) SaveContext(apiKey string, ctx *flagmint.EvaluationContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contexts[apiKey] = ctx
	return nil
}
