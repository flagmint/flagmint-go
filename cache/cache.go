package cache

import (
	"time"

	flagmint "github.com/flagmint/flagmint-go"
)

// DefaultTTL is the default time-to-live for cached flags.
// Matches the JS SDK's DEFAULT_CACHE_TTL (24 * 60 * 60 * 1000 ms).
const DefaultTTL = 24 * time.Hour

// Compile-time checks that MemoryCache and NopCache implement flagmint.CacheAdapter.
var _ flagmint.CacheAdapter = (*MemoryCache)(nil)
var _ flagmint.CacheAdapter = NopCache{}
