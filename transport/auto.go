package transport

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// AutoTransport attempts to connect via WebSocket first; on failure it falls
// back to the HTTP long-polling transport. Once a transport is selected it is
// used for the lifetime of the AutoTransport.
type AutoTransport struct {
	wsEndpoint   string
	httpEndpoint string
	apiKey       string
	logger       *slog.Logger

	// cbMu guards onUpdated.
	cbMu      sync.Mutex
	onUpdated func(map[string]any)

	// active is set during Connect and used for all subsequent calls.
	active Transport
}

// NewAutoTransport creates an AutoTransport that prefers WebSocket and falls
// back to HTTP long-polling.
func NewAutoTransport(wsEndpoint, httpEndpoint, apiKey string, logger *slog.Logger) *AutoTransport {
	return &AutoTransport{
		wsEndpoint:   wsEndpoint,
		httpEndpoint: httpEndpoint,
		apiKey:       apiKey,
		logger:       logger,
	}
}

// OnFlagsUpdated registers the callback passed to whichever transport is
// eventually selected. Must be called before Connect.
func (t *AutoTransport) OnFlagsUpdated(fn func(flags map[string]any)) {
	t.cbMu.Lock()
	defer t.cbMu.Unlock()
	t.onUpdated = fn
}

// Connect attempts WebSocket first with the given evaluation context.
// If the WebSocket dial fails it transparently switches to HTTP long-polling.
// Blocks until the chosen transport is ready or ctx is cancelled.
func (t *AutoTransport) Connect(ctx context.Context, evalCtx map[string]any) error {
	t.cbMu.Lock()
	cb := t.onUpdated
	t.cbMu.Unlock()

	// Try WebSocket with a dedicated timeout for the connection attempt.
	ws := NewWebSocketTransport(t.wsEndpoint, t.apiKey, t.logger)
	if cb != nil {
		ws.OnFlagsUpdated(cb)
	}
	// Use a separate context with a reasonable timeout for WebSocket handshake.
	ws_ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ws.Connect(ws_ctx, evalCtx); err == nil {
		t.active = ws
		t.logger.Info("auto transport: using WebSocket")
		return nil
	} else {
		t.logger.Warn("auto transport: WebSocket unavailable, falling back to HTTP", "err", err)
		_ = ws.Close()
	}

	// Fall back to HTTP.
	h := NewHTTPTransport(t.httpEndpoint, t.apiKey, t.logger)
	if cb != nil {
		h.OnFlagsUpdated(cb)
	}
	if err := h.Connect(ctx, evalCtx); err != nil {
		return err
	}
	t.active = h
	t.logger.Info("auto transport: using HTTP long-polling")
	return nil
}

// FetchFlags delegates to the active transport.
func (t *AutoTransport) FetchFlags(ctx context.Context, evalCtx map[string]any) (map[string]any, error) {
	if t.active == nil {
		return nil, fmt.Errorf("auto transport: not connected")
	}
	return t.active.FetchFlags(ctx, evalCtx)
}

// Close closes the active transport.
func (t *AutoTransport) Close() error {
	if t.active != nil {
		return t.active.Close()
	}
	return nil
}

// Ensure AutoTransport satisfies the Transport interface at compile time.
var _ Transport = (*AutoTransport)(nil)
