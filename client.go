package flagmint

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/flagmint/flagmint-go/cache"
	"github.com/flagmint/flagmint-go/internal/syncutil"
	"github.com/flagmint/flagmint-go/transport"
)

// FlagClient is the main entry point for interacting with the Flagmint feature
// flag service. Create one via [NewClient].
type FlagClient struct {
	cfg       clientConfig
	flags     syncutil.RWValue[FeatureFlags]
	evalCtx   syncutil.RWValue[*EvaluationContext]
	transport transport.Transport
	cache     cache.Adapter
	logger    *slog.Logger
}

// NewClient creates and (optionally) initialises a FlagClient.
//
// apiKey is required. Provide zero or more [Option] values to customise
// behaviour.
//
//	client, err := flagmint.NewClient("your-api-key",
//	    flagmint.WithContext(flagmint.EvaluationContext{Kind: "user", Key: "u123"}),
//	)
func NewClient(apiKey string, opts ...Option) (*FlagClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("flagmint: apiKey must not be empty")
	}

	cfg := defaultConfig()
	cfg.apiKey = apiKey
	for _, o := range opts {
		o(&cfg)
	}

	c := &FlagClient{
		cfg:    cfg,
		logger: cfg.logger,
	}

	if cfg.context != nil {
		c.evalCtx.Store(cfg.context)
	}

	// Set up cache.
	if cfg.enableCache {
		if cfg.cacheAdapter != nil {
			c.cache = cfg.cacheAdapter
		} else {
			c.cache = cache.NewMemoryCache()
		}
	}

	// Set up transport.
	switch cfg.transportMode {
	case TransportWebSocket:
		c.transport = transport.NewWebSocketTransport(cfg.wsEndpoint, apiKey, cfg.logger)
	case TransportLongPolling:
		c.transport = transport.NewHTTPTransport(cfg.restEndpoint, apiKey, cfg.logger)
	default: // TransportAuto — prefer WebSocket
		c.transport = transport.NewWebSocketTransport(cfg.wsEndpoint, apiKey, cfg.logger)
	}

	c.transport.OnFlagsUpdated(c.handleFlagsUpdated)

	if !cfg.deferInit {
		if err := c.Initialize(context.Background()); err != nil {
			return nil, err
		}
	}

	return c, nil
}

// Initialize connects the underlying transport to the Flagmint backend.
// It is called automatically by [NewClient] unless [WithDeferInit] was used.
func (c *FlagClient) Initialize(ctx context.Context) error {
	c.logger.Info("flagmint: initialising client")
	return c.transport.Connect(ctx)
}

// GetFlags returns the full set of evaluated feature flags for the current
// evaluation context.
func (c *FlagClient) GetFlags() FeatureFlags {
	return c.flags.Load()
}

// Bool returns the boolean value of flag key, or fallback if the flag is absent
// or not a bool.
func (c *FlagClient) Bool(key string, fallback bool) bool {
	return c.GetFlags().Bool(key, fallback)
}

// String returns the string value of flag key, or fallback if the flag is absent
// or not a string.
func (c *FlagClient) String(key string, fallback string) string {
	return c.GetFlags().String(key, fallback)
}

// Float64 returns the numeric value of flag key, or fallback if the flag is absent
// or not a float64.
func (c *FlagClient) Float64(key string, fallback float64) float64 {
	return c.GetFlags().Float64(key, fallback)
}

// JSON unmarshals a JSON flag configuration into target (must be a pointer).
// Returns [ErrFlagNotFound] when the key is absent.
func (c *FlagClient) JSON(key string, target any) error {
	return c.GetFlags().JSON(key, target)
}

// SetContext updates the evaluation context and triggers a flag refresh.
func (c *FlagClient) SetContext(ctx EvaluationContext) {
	c.evalCtx.Store(&ctx)
	// Invalidate cache for the new context key.
	if c.cache != nil {
		c.cache.Delete(ctx.Key)
	}
}

// Close shuts down the client, releasing all resources.
func (c *FlagClient) Close() error {
	return c.transport.Close()
}

// handleFlagsUpdated is called by the transport whenever new flags arrive.
func (c *FlagClient) handleFlagsUpdated(raw map[string]any) {
	flags := NewFeatureFlags(raw)
	c.flags.Store(flags)
	c.logger.Info("flagmint: flags updated", "count", flags.Len())

	if c.cache != nil {
		if ctx := c.evalCtx.Load(); ctx != nil {
			c.cache.Set(ctx.Key, flags)
		}
	}
}
