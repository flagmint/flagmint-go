package flagmint

import (
	"log/slog"
	"os"
)

// TransportMode controls the transport mechanism used by the client.
type TransportMode string

const (
	// TransportAuto selects WebSocket when available, falling back to long-polling.
	TransportAuto TransportMode = "auto"
	// TransportWebSocket forces the WebSocket transport.
	TransportWebSocket TransportMode = "websocket"
	// TransportLongPolling forces the HTTP long-polling transport.
	TransportLongPolling TransportMode = "long-polling"
)

// Environment variable names
const (
	EnvFlagmintRestEndpoint = "FLAGMINT_REST_ENDPOINT"
	EnvFlagmintWSEndpoint   = "FLAGMINT_WS_ENDPOINT"
	EnvFlagmintEnv          = "FLAGMINT_ENV"
)

// Environment-specific endpoint defaults
var envEndpoints = map[string][2]string{
	"local":   {"http://localhost:3000/evaluator/evaluate", "ws://localhost:3000"},
	"staging": {"https://staging-api.flagmint.com", "wss://staging-api.flagmint.com"},
	"prod":    {"https://api.flagmint.com", "wss://api.flagmint.com"},
}

// Option configures the FlagClient. Use With* functions to create options.
type Option func(*clientConfig)

type clientConfig struct {
	apiKey          string
	context         *EvaluationContext
	transportMode   TransportMode
	enableCache     bool
	cacheAdapter    CacheAdapter
	onError         func(error)
	restEndpoint    string
	wsEndpoint      string
	deferInit       bool
	logger          *slog.Logger
	localEvaluation bool
}

func defaultConfig() clientConfig {
	rest, ws := getEndpoints()
	return clientConfig{
		transportMode: TransportAuto,
		restEndpoint:  rest,
		wsEndpoint:    ws,
		logger:        slog.Default(),
	}
}

// getEndpoints returns REST and WebSocket endpoints, checking environment
// variables and FLAGMINT_ENV in order of precedence.
func getEndpoints() (rest, ws string) {
	// 1. Check for explicit endpoint overrides
	if rest = os.Getenv(EnvFlagmintRestEndpoint); rest != "" {
		ws = os.Getenv(EnvFlagmintWSEndpoint)
		if ws == "" {
			// If only REST is set, derive WS from it
			ws = "wss://" + rest[8:] // Remove "https://" and add "wss://"
		}
		return
	}

	// 2. Check for environment name (local, staging, prod)
	if env := os.Getenv(EnvFlagmintEnv); env != "" {
		if endpoints, ok := envEndpoints[env]; ok {
			return endpoints[0], endpoints[1]
		}
	}

	// 3. Default to production
	return envEndpoints["local"][0], envEndpoints["local"][1]
}

// WithContext sets the default evaluation context for the client.
func WithContext(ctx EvaluationContext) Option {
	return func(c *clientConfig) {
		c.context = &ctx
	}
}

// WithTransportMode sets the transport mechanism.
func WithTransportMode(mode TransportMode) Option {
	return func(c *clientConfig) {
		c.transportMode = mode
	}
}

// WithCache enables or disables the flag cache.
func WithCache(enabled bool) Option {
	return func(c *clientConfig) {
		c.enableCache = enabled
	}
}

// WithCacheAdapter sets a custom cache adapter. Implementations must be
// safe for concurrent use. Enables caching automatically.
func WithCacheAdapter(adapter CacheAdapter) Option {
	return func(c *clientConfig) {
		c.cacheAdapter = adapter
		c.enableCache = true
	}
}

// WithOnError registers a callback invoked when the client encounters a
// non-fatal error (e.g., a failed flag refresh).
func WithOnError(fn func(error)) Option {
	return func(c *clientConfig) {
		c.onError = fn
	}
}

// WithEndpoints overrides the default REST and WebSocket API endpoints.
func WithEndpoints(rest, ws string) Option {
	return func(c *clientConfig) {
		c.restEndpoint = rest
		c.wsEndpoint = ws
	}
}

// WithDeferInit prevents the client from connecting immediately on creation.
// Call [FlagClient.Initialize] manually when ready.
func WithDeferInit() Option {
	return func(c *clientConfig) {
		c.deferInit = true
	}
}

// WithLogger sets the structured logger used by the client.
func WithLogger(l *slog.Logger) Option {
	return func(c *clientConfig) {
		c.logger = l
	}
}

// WithLocalEvaluation enables local flag evaluation mode.
// When active, the client evaluates flags locally using FlagConfig objects
// instead of consuming pre-evaluated values from the server. This eliminates
// network round-trips for each flag check and keeps user context on-premise.
//
// In local evaluation mode, call [FlagClient.SetFlagConfigs] to supply the
// flag rule configuration. The transport integration (auto-fetching configs
// from /evaluator/config) is tracked separately.
func WithLocalEvaluation() Option {
	return func(c *clientConfig) {
		c.localEvaluation = true
	}
}
