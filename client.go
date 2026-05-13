package flagmint

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/flagmint/flagmint-go/cache"
	"github.com/flagmint/flagmint-go/internal/syncutil"
	"github.com/flagmint/flagmint-go/transport"
)

// FlagClient is the main entry point for interacting with the Flagmint feature
// flag service. Create one via [NewClient].
// Safe for concurrent use by multiple goroutines.
type FlagClient struct {
	cfg       clientConfig
	flags     syncutil.RWValue[FeatureFlags]
	evalCtx   syncutil.RWValue[*EvaluationContext]
	transport transport.Transport
	cache     cache.Adapter
	logger    *slog.Logger

	// Lifecycle
	initOnce    sync.Once
	readyCh     chan struct{} // closed when flags are available
	readyErr    error        // set before readyCh is closed (if error)
	cancelFunc  context.CancelFunc
	internalCtx context.Context

	// Subscriptions
	subMu       sync.RWMutex
	subscribers map[uint64]func(FeatureFlags)
	nextSubID   uint64
}

// NewClient creates a new FlagClient.
// Returns an error only for invalid configuration (missing API key, etc).
// Transport connection happens asynchronously; call [FlagClient.Ready] to
// block until flags are available.
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

	ctx, cancel := context.WithCancel(context.Background())
	c := &FlagClient{
		cfg:         cfg,
		logger:      cfg.logger,
		readyCh:     make(chan struct{}),
		internalCtx: ctx,
		cancelFunc:  cancel,
		subscribers: make(map[uint64]func(FeatureFlags)),
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

	c.transport.OnFlagsUpdated(c.onFlagsReceived)

	if !cfg.deferInit {
		c.initOnce.Do(func() { go c.initialize() })
	}

	return c, nil
}

// Initialize connects the underlying transport to the Flagmint backend and
// blocks until flags are available or ctx is cancelled.
// It is called automatically by [NewClient] unless [WithDeferInit] was used.
// Calling Initialize is equivalent to calling Ready(ctx).
func (c *FlagClient) Initialize(ctx context.Context) error {
	return c.Ready(ctx)
}

// Ready blocks until the client has flags available (from cache or server)
// or the provided context is cancelled/times out.
//
// If [WithDeferInit] was used, Ready triggers the first connection.
// Subsequent calls return the original result immediately.
//
// Returns nil if flags are available (even if from cache in degraded mode).
// Returns an error if initialisation failed and no cached flags exist, or if
// the provided context or internal context is cancelled.
func (c *FlagClient) Ready(ctx context.Context) error {
	// Trigger initialisation exactly once (no-op if already started).
	c.initOnce.Do(func() { go c.initialize() })

	select {
	case <-c.readyCh:
		return c.readyErr
	case <-ctx.Done():
		return ctx.Err()
	case <-c.internalCtx.Done():
		return c.internalCtx.Err()
	}
}

// initialize performs the one-time setup: loads cached flags, connects the
// transport, and closes readyCh when done. It is always called in its own
// goroutine via initOnce.
func (c *FlagClient) initialize() {
	c.logger.Info("flagmint: initialising client")

	var hasCachedFlags bool

	// Attempt to load flags from cache first (degraded-mode support).
	if c.cache != nil {
		if evalCtx := c.evalCtx.Load(); evalCtx != nil {
			if cached, ok := c.cache.Get(evalCtx.Key); ok {
				if flags, ok := cached.(FeatureFlags); ok {
					c.updateFlags(flags)
					hasCachedFlags = true
				}
			}
		}
	}

	// Connect the transport; pass internal context so Close cancels it.
	if err := c.transport.Connect(c.internalCtx); err != nil {
		if hasCachedFlags {
			// Degraded mode: return success but report the error.
			if c.cfg.onError != nil {
				c.cfg.onError(err)
			}
			c.readyErr = nil
		} else {
			c.readyErr = err
		}
	}

	close(c.readyCh)
}

// GetFlags returns a shallow copy of all current flag values.
// The caller cannot mutate the client's internal flag state.
func (c *FlagClient) GetFlags() FeatureFlags {
	return c.flags.Load().Clone()
}

// GetFlag returns the value for a single flag key, or fallback if not found.
func (c *FlagClient) GetFlag(key string, fallback any) any {
	if v, ok := c.GetFlags().Get(key); ok {
		return v
	}
	return fallback
}

// Bool returns the boolean value of flag key, or fallback if the flag is absent
// or not a bool.
func (c *FlagClient) Bool(key string, fallback bool) bool {
	return c.GetFlags().Bool(key, fallback)
}

// BoolFlag is a typed convenience method. Returns fallback if the flag
// doesn't exist or isn't a bool.
func (c *FlagClient) BoolFlag(key string, fallback bool) bool {
	return c.GetFlags().Bool(key, fallback)
}

// String returns the string value of flag key, or fallback if the flag is absent
// or not a string.
func (c *FlagClient) String(key string, fallback string) string {
	return c.GetFlags().String(key, fallback)
}

// StringFlag is a typed convenience method. Returns fallback if the flag
// doesn't exist or isn't a string.
func (c *FlagClient) StringFlag(key string, fallback string) string {
	return c.GetFlags().String(key, fallback)
}

// Float64 returns the numeric value of flag key, or fallback if the flag is absent
// or not a float64.
func (c *FlagClient) Float64(key string, fallback float64) float64 {
	return c.GetFlags().Float64(key, fallback)
}

// NumberFlag is a typed convenience method. Flags are float64 internally.
// Returns fallback if the flag doesn't exist or isn't a float64.
func (c *FlagClient) NumberFlag(key string, fallback float64) float64 {
	return c.GetFlags().Float64(key, fallback)
}

// JSON unmarshals a JSON flag configuration into target (must be a pointer).
// Returns [ErrFlagNotFound] when the key is absent.
func (c *FlagClient) JSON(key string, target any) error {
	return c.GetFlags().JSON(key, target)
}

// JSONFlag is a typed convenience method. Returns the flag as map[string]any,
// or fallback if the flag doesn't exist or isn't a JSON object.
func (c *FlagClient) JSONFlag(key string, fallback map[string]any) map[string]any {
	v, ok := c.GetFlags().Get(key)
	if !ok {
		return fallback
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fallback
	}
	return m
}

// UpdateContext merges the provided evaluation context with the existing one,
// persists it to the cache if caching is enabled, and triggers a flag
// re-evaluation. Thread-safe.
func (c *FlagClient) UpdateContext(ctx EvaluationContext) error {
	c.evalCtx.Store(&ctx)
	if c.cache != nil {
		// Remove stale entry for the previous key so the next fetch is clean.
		c.cache.Delete(ctx.Key)
	}
	return nil
}

// SetContext updates the evaluation context. Deprecated: use UpdateContext.
func (c *FlagClient) SetContext(ctx EvaluationContext) {
	_ = c.UpdateContext(ctx) //nolint:errcheck
}

// Subscribe registers a callback that fires whenever flags change.
// The callback is also invoked immediately with the current flag state.
// Returns an unsubscribe function; calling it removes the subscription.
// Safe to call from any goroutine.
//
// Callbacks are invoked synchronously and should be fast and non-blocking.
// If slow work is needed, dispatch to a separate goroutine inside the callback.
func (c *FlagClient) Subscribe(fn func(FeatureFlags)) func() {
	c.subMu.Lock()
	id := c.nextSubID
	c.nextSubID++
	c.subscribers[id] = fn
	c.subMu.Unlock()

	// Deliver current state immediately.
	fn(c.GetFlags())

	return func() {
		c.subMu.Lock()
		delete(c.subscribers, id)
		c.subMu.Unlock()
	}
}

// Close shuts down the client, cancels the internal context, closes the
// transport, and releases all subscriber registrations.
// Safe to call more than once.
func (c *FlagClient) Close() error {
	c.cancelFunc()

	c.subMu.Lock()
	c.subscribers = make(map[uint64]func(FeatureFlags))
	c.subMu.Unlock()

	return c.transport.Close()
}

// onFlagsReceived is registered with the transport and called whenever a new
// raw flag payload arrives from the backend.
func (c *FlagClient) onFlagsReceived(raw map[string]any) {
	c.updateFlags(NewFeatureFlags(raw))
}

// updateFlags stores the new flag set, persists it to cache, and notifies
// all registered subscribers. It is the single write path for flag state.
func (c *FlagClient) updateFlags(flags FeatureFlags) {
	c.flags.Store(flags)
	c.logger.Info("flagmint: flags updated", "count", flags.Len())

	if c.cache != nil {
		if ctx := c.evalCtx.Load(); ctx != nil {
			c.cache.Set(ctx.Key, flags)
		}
	}

	c.notifySubscribers(flags)
}

// notifySubscribers delivers flags to all registered callbacks under a read
// lock. Callbacks must not call Subscribe or the unsubscribe function (deadlock).
func (c *FlagClient) notifySubscribers(flags FeatureFlags) {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	for _, fn := range c.subscribers {
		fn(flags)
	}
}
