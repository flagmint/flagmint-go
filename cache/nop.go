package cache

import (
	flagmint "github.com/flagmint/flagmint-go"
)

// NopCache is a no-op CacheAdapter that discards all writes and returns nil
// on all reads. Use it when caching is disabled.
type NopCache struct{}

// LoadFlags always returns nil, nil (cache miss).
func (NopCache) LoadFlags(_ string) (*flagmint.FeatureFlags, error) { return nil, nil }

// SaveFlags discards the flags without storing them.
func (NopCache) SaveFlags(_ string, _ flagmint.FeatureFlags) error { return nil }

// LoadContext always returns nil, nil (cache miss).
func (NopCache) LoadContext(_ string) (*flagmint.EvaluationContext, error) { return nil, nil }

// SaveContext discards the context without storing it.
func (NopCache) SaveContext(_ string, _ *flagmint.EvaluationContext) error { return nil }
