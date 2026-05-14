package flagmint

import (
	"log/slog"
	"os"

	"github.com/flagmint/flagmint-go/cache"
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
	"local":   {"http://localhost:3000", "ws://localhost:3000"},
	"staging": {"https://staging-api.flagmint.com", "wss://staging-api.flagmint.com"},
	"prod":    {"https://api.flagmint.com", "wss://api.flagmint.com"},
}

// Option configures the FlagClient. Use With* functions to create options.
type Option func(*clientConfig)

type clientConfig struct {
	apiKey        string
	context       *EvaluationContext
	transportMode TransportMode
	enableCache   bool
	cacheAdapter  cache.Adapter
	onError       func(error)
	restEndpoint  string
	wsEndpoint    string
	deferInit     bool
	logger        *slog.Logger
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
	return envEndpoints["prod"][0], envEndpoints["prod"][1]
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

// WithCacheAdapter sets a custom cache adapter.
func WithCacheAdapter(adapter cache.Adapter) Option {
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
