//go:build ignore

// Package main demonstrates a Redis-backed CacheAdapter for the Flagmint Go SDK.
//
// The adapter satisfies the flagmint.CacheAdapter interface and stores flag
// data in Redis, enabling shared caching across multiple server instances
// and fast cold-start flag availability (degraded-mode support).
//
// Prerequisites:
//
//	go get github.com/redis/go-redis/v9
//
// Run a local Redis instance first, then:
//
//	go run main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	flagmint "github.com/flagmint/flagmint-go"
)

const (
	apiKey          = "fm_sdk_your_api_key"
	redisFlagsKey   = "flagmint:flags:"
	redisContextKey = "flagmint:ctx:"
	flagsTTL        = 24 * time.Hour
)

// RedisCacheAdapter is a flagmint.CacheAdapter backed by Redis.
// It implements all four methods of the interface using JSON encoding.
// All methods are safe for concurrent use.
type RedisCacheAdapter struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCacheAdapter creates a new adapter that connects to the given Redis
// address (e.g. "localhost:6379").
func NewRedisCacheAdapter(addr string, ttl time.Duration) *RedisCacheAdapter {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &RedisCacheAdapter{client: rdb, ttl: ttl}
}

// LoadFlags implements flagmint.CacheAdapter. Returns nil when the key is
// absent or has expired.
func (r *RedisCacheAdapter) LoadFlags(apiKey string) (*flagmint.FeatureFlags, error) {
	ctx := context.Background()
	raw, err := r.client.Get(ctx, redisFlagsKey+apiKey).Bytes()
	if err == redis.Nil {
		return nil, nil // cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("redis LoadFlags: %w", err)
	}

	var flags flagmint.FeatureFlags
	if err := json.Unmarshal(raw, &flags); err != nil {
		return nil, fmt.Errorf("redis LoadFlags unmarshal: %w", err)
	}
	return &flags, nil
}

// SaveFlags implements flagmint.CacheAdapter. Serialises flags as JSON and
// stores them with the configured TTL.
func (r *RedisCacheAdapter) SaveFlags(apiKey string, flags flagmint.FeatureFlags) error {
	raw, err := json.Marshal(flags)
	if err != nil {
		return fmt.Errorf("redis SaveFlags marshal: %w", err)
	}

	ctx := context.Background()
	if err := r.client.Set(ctx, redisFlagsKey+apiKey, raw, r.ttl).Err(); err != nil {
		return fmt.Errorf("redis SaveFlags: %w", err)
	}
	return nil
}

// LoadContext implements flagmint.CacheAdapter.
func (r *RedisCacheAdapter) LoadContext(apiKey string) (*flagmint.EvaluationContext, error) {
	ctx := context.Background()
	raw, err := r.client.Get(ctx, redisContextKey+apiKey).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis LoadContext: %w", err)
	}

	var evalCtx flagmint.EvaluationContext
	if err := json.Unmarshal(raw, &evalCtx); err != nil {
		return nil, fmt.Errorf("redis LoadContext unmarshal: %w", err)
	}
	return &evalCtx, nil
}

// SaveContext implements flagmint.CacheAdapter.
func (r *RedisCacheAdapter) SaveContext(apiKey string, evalCtx *flagmint.EvaluationContext) error {
	raw, err := json.Marshal(evalCtx)
	if err != nil {
		return fmt.Errorf("redis SaveContext marshal: %w", err)
	}

	ctx := context.Background()
	// Context has no TTL — it persists until explicitly overwritten.
	if err := r.client.Set(ctx, redisContextKey+apiKey, raw, 0).Err(); err != nil {
		return fmt.Errorf("redis SaveContext: %w", err)
	}
	return nil
}

func main() {
	adapter := NewRedisCacheAdapter("localhost:6379", flagsTTL)

	client, err := flagmint.NewClient(apiKey,
		flagmint.WithContext(flagmint.EvaluationContext{
			Kind: "user",
			Key:  "user-123",
			Attributes: map[string]any{
				"plan": "pro",
			},
		}),
		// Use the Redis-backed adapter instead of the default in-memory cache.
		flagmint.WithCacheAdapter(adapter),
		flagmint.WithOnError(func(err error) {
			log.Printf("flagmint error: %v", err)
		}),
	)
	if err != nil {
		log.Fatal("failed to create flagmint client:", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("flagmint close error: %v", err)
		}
	}()

	// If flags were previously cached in Redis, they are available immediately
	// even before the transport connects (degraded-mode).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ready(ctx); err != nil {
		log.Printf("flagmint not ready (using cached/default values): %v", err)
	}

	fmt.Println("dark-mode:", client.BoolFlag("dark-mode", false))
	fmt.Println("max-upload-mb:", client.NumberFlag("max-upload-mb", 10))
}
