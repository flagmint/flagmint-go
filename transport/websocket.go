package transport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/flagmint/flagmint-go/internal/backoff"
)

const (
	wsPath              = "/ws/sdk"
	defaultFetchTimeout = 10 * time.Second
)

// wsMessage is the wire format for WebSocket messages.
type wsMessage struct {
	Type    string         `json:"type"`
	Context map[string]any `json:"context,omitempty"`
	Flags   map[string]any `json:"flags,omitempty"`
	Key     string         `json:"key,omitempty"`
	Value   any            `json:"value,omitempty"`
}

// WebSocketTransport connects to the Flagmint backend over a persistent
// WebSocket connection and streams flag updates in real time.
// Safe for concurrent use after Connect() returns.
type WebSocketTransport struct {
	endpoint string
	apiKey   string
	logger   *slog.Logger

	// mu guards conn, lastEvalCtx and pendingCh.
	mu          sync.Mutex
	conn        *websocket.Conn
	lastEvalCtx map[string]any
	pendingCh   chan map[string]any // awaiting FetchFlags response

	// writeMu serialises writes so readLoop reconnect and FetchFlags don't race.
	writeMu sync.Mutex

	// onUpdated is set by OnFlagsUpdated and read from readLoop; guarded by cbMu.
	cbMu      sync.Mutex
	onUpdated func(map[string]any)

	// Lifecycle
	innerCtx    context.Context
	innerCancel context.CancelFunc
	closeOnce   sync.Once
	wg          sync.WaitGroup
}

// NewWebSocketTransport creates a new WebSocketTransport.
func NewWebSocketTransport(endpoint, apiKey string, logger *slog.Logger) *WebSocketTransport {
	return &WebSocketTransport{
		endpoint: endpoint,
		apiKey:   apiKey,
		logger:   logger,
	}
}

// OnFlagsUpdated registers the callback invoked when the server pushes flag updates.
// Must be called before Connect.
func (t *WebSocketTransport) OnFlagsUpdated(fn func(flags map[string]any)) {
	t.cbMu.Lock()
	defer t.cbMu.Unlock()
	t.onUpdated = fn
}

// Connect establishes the WebSocket connection with optional initial context and starts the background read loop.
// If evalCtx is provided and non-empty, it is sent in the x-flagmint-context header.
// It blocks until the handshake completes or ctx is cancelled.
func (t *WebSocketTransport) Connect(ctx context.Context, evalCtx map[string]any) error {
	conn, err := t.dial(ctx, evalCtx)
	if err != nil {
		return fmt.Errorf("websocket: connect: %w", err)
	}

	inner, cancel := context.WithCancel(context.Background())
	t.innerCtx = inner
	t.innerCancel = cancel

	t.mu.Lock()
	t.conn = conn
	if len(evalCtx) > 0 {
		t.lastEvalCtx = evalCtx
	}
	t.mu.Unlock()

	t.wg.Add(1)
	go t.readLoop(inner, conn)

	t.logger.Info("websocket transport: connected", "endpoint", t.wsURL())
	return nil
}

// FetchFlags sends the current evaluation context to the server (via header if changed)
// and waits for the flag response. It uses the context to impose a deadline.
func (t *WebSocketTransport) FetchFlags(ctx context.Context, evalCtx map[string]any) (map[string]any, error) {
	// Remember for reconnect re-sends.
	replyCh := make(chan map[string]any, 1)

	t.mu.Lock()
	prevCtx := t.lastEvalCtx
	t.lastEvalCtx = evalCtx
	t.pendingCh = replyCh
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("websocket: not connected")
	}

	t.logger.Debug("FetchFlags: context updated", "prev", prevCtx, "new", evalCtx)

	// If context changed and new context is not empty, reconnect to send via header
	if !contextsEqual(prevCtx, evalCtx) && len(evalCtx) > 0 {
		t.logger.Debug("FetchFlags: context changed, reconnecting to send via header")
		bo := &backoff.Backoff{
			Base:       time.Second,
			Multiplier: 2.0,
			MaxDelay:   60 * time.Second,
			Jitter:     0.2,
		}
		newConn := t.reconnect(ctx, bo, evalCtx)
		if newConn == nil {
			t.mu.Lock()
			if t.pendingCh == replyCh {
				t.pendingCh = nil
			}
			t.mu.Unlock()
			return nil, fmt.Errorf("websocket: FetchFlags: reconnect cancelled")
		}
		conn = newConn
	}

	// Wait for the readLoop to deliver the response.
	fetchCtx, cancel := context.WithTimeout(ctx, defaultFetchTimeout)
	defer cancel()

	select {
	case flags := <-replyCh:
		return flags, nil
	case <-fetchCtx.Done():
		t.mu.Lock()
		if t.pendingCh == replyCh {
			t.pendingCh = nil
		}
		t.mu.Unlock()
		return nil, fmt.Errorf("websocket: FetchFlags: timeout waiting for response: %w", fetchCtx.Err())
	}
}

// Close shuts down the WebSocket connection and waits for all goroutines to exit.
// Safe to call multiple times.
func (t *WebSocketTransport) Close() error {
	t.closeOnce.Do(func() {
		if t.innerCancel != nil {
			t.innerCancel()
		}
		t.mu.Lock()
		conn := t.conn
		t.mu.Unlock()
		if conn != nil {
			_ = conn.Close(websocket.StatusNormalClosure, "client closed")
		}
	})
	t.wg.Wait()
	return nil
}

// wsURL returns the full WebSocket endpoint URL.
func (t *WebSocketTransport) wsURL() string {
	return t.endpoint + wsPath + "?apiKey=" + url.QueryEscape(t.apiKey)
}

// dial opens a new WebSocket connection, passing the API key as a header.
// If evalCtx is provided and non-empty, it will be sent as the x-flagmint-context header.
func (t *WebSocketTransport) dial(ctx context.Context, evalCtx map[string]any) (*websocket.Conn, error) {
	wsURL := t.wsURL()
	t.logger.Info("websocket: dialing", "url", wsURL)
	headers := http.Header{
		"x-api-key": {t.apiKey},
	}

	// Add context as header if it exists and is not empty
	if len(evalCtx) > 0 {
		contextJSON, err := json.Marshal(evalCtx)
		if err == nil {
			// Base64 encode for safe header transmission
			// Base64 characters (A-Z, a-z, 0-9, +, /, =) are safe for HTTP headers
			contextB64 := base64.StdEncoding.EncodeToString(contextJSON)
			// Use x-flagmint-context to match server expectation and avoid auto-encoding
			headers["x-flagmint-context"] = []string{contextB64}
			t.logger.Debug("sending context via header", "base64", contextB64)
		} else {
			t.logger.Warn("failed to marshal context for header", "err", err)
		}
	}

	// Add rate limit bypass token if available
	if token := os.Getenv("RATE_LIMIT_BYPASS_TOKEN"); token != "" {
		headers.Set("x-bypass-rate-limit", token)
	}
	opts := &websocket.DialOptions{
		HTTPHeader: headers,
	}
	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	return conn, err
}

// readLoop reads messages from conn in a loop, dispatching flags updates and
// reconnecting on transient errors.
func (t *WebSocketTransport) readLoop(ctx context.Context, conn *websocket.Conn) {
	defer t.wg.Done()

	bo := &backoff.Backoff{
		Base:       time.Second,
		Multiplier: 2.0,
		MaxDelay:   60 * time.Second,
		Jitter:     0.2,
	}

	for {
		var msg wsMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			if ctx.Err() != nil {
				// Context cancelled — clean shutdown.
				return
			}
			t.logger.Warn("websocket: read error, reconnecting", "err", err)

			t.mu.Lock()
			t.conn = nil
			t.mu.Unlock()

			// Get last context before reconnecting (will be sent as header)
			t.mu.Lock()
			lastCtx := t.lastEvalCtx
			t.mu.Unlock()

			// Reconnect loop with exponential backoff.
			// Pass lastCtx as header on reconnect (more efficient than message)
			conn = t.reconnect(ctx, bo, lastCtx)
			if conn == nil {
				// ctx was cancelled during reconnect.
				return
			}

			t.mu.Lock()
			t.conn = conn
			t.mu.Unlock()

			// Context is now sent as header on reconnect, no need for message-based resend
			if lastCtx != nil {
				t.logger.Info("reconnected with context in header", "context", lastCtx)
			}
			continue
		}

		// Log the raw message received
		rawJSON, _ := json.Marshal(msg)
		t.logger.Debug("raw message from WebSocket", "json", string(rawJSON))

		bo.Reset()
		t.handleMessage(ctx, conn, &msg)
	}
}

// reconnect attempts to re-establish the WebSocket connection using exponential
// backoff. Returns nil if ctx is cancelled before a successful dial.
// If lastContext is provided, it will be sent as a header on the new connection.
func (t *WebSocketTransport) reconnect(ctx context.Context, bo *backoff.Backoff, lastContext map[string]any) *websocket.Conn {
	for {
		delay := bo.Next()
		t.logger.Info("websocket: reconnecting", "delay", delay)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}

		conn, err := t.dial(ctx, lastContext)
		if err != nil {
			t.logger.Warn("websocket: reconnect dial failed", "err", err)
			continue
		}
		t.logger.Info("websocket: reconnected")
		return conn
	}
}

// handleMessage processes a single inbound server message.
func (t *WebSocketTransport) handleMessage(ctx context.Context, conn *websocket.Conn, msg *wsMessage) {
	t.logger.Debug("received message from API", "type", msg.Type, "message", msg)

	switch msg.Type {
	case "flags":
		t.logger.Debug("received flags from API", "flags", msg.Flags, "count", len(msg.Flags))
		t.deliverFlags(msg.Flags)
	case "flag_update":
		// Delta update (future, Ticket 4): apply to current flags and deliver.
		if msg.Key != "" {
			t.mu.Lock()
			// Build a synthetic full-flags map from the last known state.
			// For now just deliver a single-flag map so callers are notified.
			t.mu.Unlock()
			t.deliverFlags(map[string]any{msg.Key: msg.Value})
		}
	case "ping":

		// Server sent a ping; respond with pong to keep connection alive.
		// This is critical for long-lived connections.
		t.logger.Debug("received ping from server, sending pong")
		pong := map[string]any{"type": "pong"}
		t.writeMu.Lock()
		err := wsjson.Write(ctx, conn, pong)
		t.writeMu.Unlock()
		if err != nil {
			t.logger.Warn("failed to send pong to server", "err", err)
		}
	case "connection.status":
		// Server sent connection status update; just log and ignore.
		// This is typically a welcome message on connect.
		t.logger.Debug("received connection.status from server", "payload", msg)
	default:
		t.logger.Debug("websocket: unknown message type", "type", msg.Type, "message", msg)
	}
}

// deliverFlags notifies the pending FetchFlags waiter (if any) and calls the
// onUpdated callback.
func (t *WebSocketTransport) deliverFlags(flags map[string]any) {
	if flags == nil {
		return
	}

	// Clone the map so internal state cannot be mutated by callers.
	clone := make(map[string]any, len(flags))
	for k, v := range flags {
		clone[k] = v
	}

	t.mu.Lock()
	pending := t.pendingCh
	t.pendingCh = nil
	t.mu.Unlock()

	if pending != nil {
		select {
		case pending <- clone:
		default:
		}
	}

	t.cbMu.Lock()
	fn := t.onUpdated
	t.cbMu.Unlock()

	if fn != nil {
		fn(clone)
	}
}

// contextsEqual compares two evaluation contexts for equality.
func contextsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || v != bv {
			return false
		}
	}
	return true
}

// Ensure WebSocketTransport satisfies the Transport interface at compile time.
var _ Transport = (*WebSocketTransport)(nil)
