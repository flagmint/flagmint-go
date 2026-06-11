package flagmint

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/flagmint/flagmint-go/evaluate"
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
	cache     CacheAdapter
	logger    *slog.Logger

	// Local evaluation
	evaluator     *evaluate.Evaluator
	flagConfigsMu sync.RWMutex
	flagConfigs   map[string]*evaluate.FlagConfig

	// Lifecycle
	initOnce    sync.Once
	readyCh     chan struct{} // closed when flags are available
	readyErr    error         // set before readyCh is closed (if error)
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

	// Set up local evaluation.
	if cfg.localEvaluation {
		c.flagConfigs = make(map[string]*evaluate.FlagConfig)
		c.evaluator = evaluate.NewEvaluatorWithLogger(cfg.logger)
	}

	// Set up cache.
	if cfg.enableCache {
		if cfg.cacheAdapter != nil {
			c.cache = cfg.cacheAdapter
		} else {
			c.cache = newDefaultMemoryCache()
		}
	}

	// Set up transport.
	switch cfg.transportMode {
	case TransportWebSocket:
		c.transport = transport.NewWebSocketTransport(cfg.wsEndpoint, apiKey, cfg.logger)
	case TransportLongPolling:
		c.transport = transport.NewHTTPTransport(cfg.restEndpoint, apiKey, cfg.logger)
	default: // TransportAuto — prefer WebSocket, fall back to HTTP
		c.transport = transport.NewAutoTransport(cfg.wsEndpoint, cfg.restEndpoint, apiKey, cfg.logger)
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

// initialize performs the one-time setup: loads cached context and flags,
// connects the transport, and closes readyCh when done. It is always called
// in its own goroutine via initOnce.
func (c *FlagClient) initialize() {
	c.logger.Info("flagmint: initialising client")

	var hasCachedFlags bool

	if c.cache != nil {
		// Step A: Restore persisted context if none was provided at construction.
		if c.evalCtx.Load() == nil {
			if cachedCtx, err := c.cache.LoadContext(c.cfg.apiKey); err == nil && cachedCtx != nil {
				c.evalCtx.Store(cachedCtx)
			} else if err != nil {
				c.logger.Warn("flagmint: failed to load cached context", "error", err)
			}
		}

		// Step B: Serve cached flags immediately (degraded-mode support).
		if cachedFlags, err := c.cache.LoadFlags(c.cfg.apiKey); err == nil && cachedFlags != nil {
			c.updateFlags(*cachedFlags)
			hasCachedFlags = true
		} else if err != nil {
			c.logger.Warn("flagmint: failed to load cached flags", "error", err)
		}
	}

	// Connect the transport with initial evaluation context.
	evalCtx := c.evalCtx.Load()
	if evalCtx == nil {
		evalCtx = &EvaluationContext{}
	}

	// Convert evaluation context to map for transport
	evalCtxMap, err := evalContextToMap(*evalCtx)
	if err != nil {
		c.logger.Warn("flagmint: failed to marshal evaluation context", "err", err)
		evalCtxMap = make(map[string]any) // Fall back to empty context
	}

	if err := c.transport.Connect(c.internalCtx, evalCtxMap); err != nil {
		if hasCachedFlags {
			// Degraded mode: return success but report the error.
			if c.cfg.onError != nil {
				c.cfg.onError(err)
			}
			c.readyErr = nil
		} else {
			c.readyErr = err
		}
	} else {
		// Fetch flags with the current evaluation context on initial connect.
		if evalCtx := c.evalCtx.Load(); evalCtx != nil {
			c.doFetchFlags(*evalCtx)
		} else {
			c.doFetchFlags(EvaluationContext{})
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
// When local evaluation is enabled (see [WithLocalEvaluation]) the flag is
// evaluated against the full FlagConfig stored via [FlagClient.SetFlagConfigs].
func (c *FlagClient) GetFlag(key string, fallback any) any {
	if c.cfg.localEvaluation {
		c.flagConfigsMu.RLock()
		config, ok := c.flagConfigs[key]
		c.flagConfigsMu.RUnlock()
		if !ok {
			return fallback
		}
		var flatCtx map[string]any
		if ec := c.evalCtx.Load(); ec != nil {
			flatCtx = ec.Flatten()
		} else {
			flatCtx = make(map[string]any)
		}
		result, err := c.evaluator.Evaluate(config, flatCtx)
		if err != nil {
			c.logger.Warn("flagmint: local evaluation failed", "flag", key, "error", err)
			return fallback
		}
		return result
	}

	if v, ok := c.GetFlags().Get(key); ok {
		return v
	}
	return fallback
}

// SetFlagConfigs replaces the local flag configuration used for local
// evaluation. Pass a map of flag key → *[evaluate.FlagConfig]. Each config's
// [evaluate.FlagConfig.HydrateVariations] must have been called before passing
// it here if it was not already hydrated.
// This method is a no-op when [WithLocalEvaluation] was not set.
func (c *FlagClient) SetFlagConfigs(configs map[string]*evaluate.FlagConfig) {
	if !c.cfg.localEvaluation {
		return
	}
	c.flagConfigsMu.Lock()
	defer c.flagConfigsMu.Unlock()
	c.flagConfigs = configs
}

// Bool returns the boolean value of flag key, or fallback if the flag is absent
// or not a bool. When local evaluation is enabled, delegates to [FlagClient.GetFlag].
func (c *FlagClient) Bool(key string, fallback bool) bool {
	if c.cfg.localEvaluation {
		v, ok := c.GetFlag(key, fallback).(bool)
		if !ok {
			return fallback
		}
		return v
	}
	return c.GetFlags().Bool(key, fallback)
}

// BoolFlag is a typed convenience method. Returns fallback if the flag
// doesn't exist or isn't a bool. When local evaluation is enabled, the flag
// is evaluated locally against the configured [evaluate.FlagConfig] rules.
func (c *FlagClient) BoolFlag(key string, fallback bool) bool {
	return c.Bool(key, fallback)
}

// String returns the string value of flag key, or fallback if the flag is absent
// or not a string. When local evaluation is enabled, delegates to [FlagClient.GetFlag].
func (c *FlagClient) String(key string, fallback string) string {
	if c.cfg.localEvaluation {
		v, ok := c.GetFlag(key, fallback).(string)
		if !ok {
			return fallback
		}
		return v
	}
	return c.GetFlags().String(key, fallback)
}

// StringFlag is a typed convenience method. Returns fallback if the flag
// doesn't exist or isn't a string. When local evaluation is enabled, the flag
// is evaluated locally against the configured [evaluate.FlagConfig] rules.
func (c *FlagClient) StringFlag(key string, fallback string) string {
	return c.String(key, fallback)
}

// Float64 returns the numeric value of flag key, or fallback if the flag is absent
// or not a float64. When local evaluation is enabled, delegates to [FlagClient.GetFlag].
func (c *FlagClient) Float64(key string, fallback float64) float64 {
	if c.cfg.localEvaluation {
		v, ok := c.GetFlag(key, fallback).(float64)
		if !ok {
			return fallback
		}
		return v
	}
	return c.GetFlags().Float64(key, fallback)
}

// NumberFlag is a typed convenience method. Flags are float64 internally.
// Returns fallback if the flag doesn't exist or isn't a float64. When local
// evaluation is enabled, the flag is evaluated locally against the configured
// [evaluate.FlagConfig] rules.
func (c *FlagClient) NumberFlag(key string, fallback float64) float64 {
	return c.Float64(key, fallback)
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
		if err := c.cache.SaveContext(c.cfg.apiKey, &ctx); err != nil {
			c.logger.Warn("flagmint: failed to save context to cache", "error", err)
		}
	}
	// Trigger a flag refresh with the new context in the background.
	go c.fetchFlagsForContext(ctx)
	return nil
}

// fetchFlagsForContext converts the EvaluationContext to a map and calls
// FetchFlags on the transport, updating client flag state on success.
// It waits for the transport to be ready before calling FetchFlags.
func (c *FlagClient) fetchFlagsForContext(ctx EvaluationContext) {
	// Wait until transport is connected (readyCh closed) or client is closed.
	select {
	case <-c.readyCh:
	case <-c.internalCtx.Done():
		return
	}
	if c.readyErr != nil {
		return
	}
	c.doFetchFlags(ctx)
}

// doFetchFlags performs the actual flag fetch without waiting for readyCh.
// Used both by fetchFlagsForContext and during initialization.
func (c *FlagClient) doFetchFlags(ctx EvaluationContext) {
	evalMap, err := evalContextToMap(ctx)
	if err != nil {
		if c.cfg.onError != nil {
			c.cfg.onError(fmt.Errorf("flagmint: marshal eval context: %w", err))
		}
		return
	}
	flags, err := c.transport.FetchFlags(c.internalCtx, evalMap)
	if err != nil {
		if c.internalCtx.Err() == nil && c.cfg.onError != nil {
			c.cfg.onError(fmt.Errorf("flagmint: FetchFlags: %w", err))
		}
		return
	}
	c.updateFlags(NewFeatureFlags(flags))
}

// evalContextToMap converts an EvaluationContext struct into a map[string]any
// via JSON round-trip so the transport layer has no dependency on this package.
func evalContextToMap(ctx EvaluationContext) (map[string]any, error) {
	b, err := json.Marshal(ctx)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
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
	c.logger.Debug("flagmint: received flags from transport", "flags", raw)
	c.updateFlags(NewFeatureFlags(raw))
}

// updateFlags stores the new flag set, persists it to cache (best-effort), and
// notifies all registered subscribers. It is the single write path for flag state.
func (c *FlagClient) updateFlags(flags FeatureFlags) {
	c.flags.Store(flags)
	c.logger.Info("flagmint: flags updated", "count", flags.Len(), "flags", flags.ToMap())

	if c.cache != nil {
		if err := c.cache.SaveFlags(c.cfg.apiKey, flags); err != nil {
			c.logger.Warn("flagmint: failed to save flags to cache", "error", err)
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
