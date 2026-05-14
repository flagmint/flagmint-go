package transport

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/flagmint/flagmint-go/internal/backoff"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	wsPath          = "/ws/sdk"
	fetchFlagsTimeout = 10 * time.Second
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

// Connect establishes the WebSocket connection and starts the background read loop.
// It blocks until the handshake completes or ctx is cancelled.
func (t *WebSocketTransport) Connect(ctx context.Context) error {
	conn, err := t.dial(ctx)
	if err != nil {
		return fmt.Errorf("websocket: connect: %w", err)
	}

	inner, cancel := context.WithCancel(context.Background())
	t.innerCtx = inner
	t.innerCancel = cancel

	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()

	t.wg.Add(1)
	go t.readLoop(inner, conn)

	t.logger.Info("websocket transport: connected", "endpoint", t.wsURL())
	return nil
}

// FetchFlags sends the current evaluation context to the server and waits for
// the flag response. It uses the context to impose a deadline.
func (t *WebSocketTransport) FetchFlags(ctx context.Context, evalCtx map[string]any) (map[string]any, error) {
	// Remember for reconnect re-sends.
	replyCh := make(chan map[string]any, 1)

	t.mu.Lock()
	t.lastEvalCtx = evalCtx
	t.pendingCh = replyCh
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("websocket: not connected")
	}

	msg := wsMessage{
		Type:    "context",
		Context: evalCtx,
	}

	t.writeMu.Lock()
	err := wsjson.Write(ctx, conn, msg)
	t.writeMu.Unlock()
	if err != nil {
		t.mu.Lock()
		if t.pendingCh == replyCh {
			t.pendingCh = nil
		}
		t.mu.Unlock()
		return nil, fmt.Errorf("websocket: FetchFlags: send: %w", err)
	}

	// Wait for the readLoop to deliver the response.
	fetchCtx, cancel := context.WithTimeout(ctx, fetchFlagsTimeout)
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
	return t.endpoint + wsPath
}

// dial opens a new WebSocket connection, passing the API key as a header.
func (t *WebSocketTransport) dial(ctx context.Context) (*websocket.Conn, error) {
	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"x-api-key": {t.apiKey},
		},
	}
	conn, _, err := websocket.Dial(ctx, t.wsURL(), opts)
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

			// Reconnect loop with exponential backoff.
			conn = t.reconnect(ctx, bo)
			if conn == nil {
				// ctx was cancelled during reconnect.
				return
			}

			t.mu.Lock()
			t.conn = conn
			// Re-send last evaluation context if present.
			lastCtx := t.lastEvalCtx
			t.mu.Unlock()

			if lastCtx != nil {
				resend := wsMessage{Type: "context", Context: lastCtx}
				t.writeMu.Lock()
				_ = wsjson.Write(ctx, conn, resend)
				t.writeMu.Unlock()
			}
			continue
		}

		bo.Reset()
		t.handleMessage(ctx, conn, &msg)
	}
}

// reconnect attempts to re-establish the WebSocket connection using exponential
// backoff. Returns nil if ctx is cancelled before a successful dial.
func (t *WebSocketTransport) reconnect(ctx context.Context, bo *backoff.Backoff) *websocket.Conn {
	for {
		delay := bo.Next()
		t.logger.Info("websocket: reconnecting", "delay", delay)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}

		conn, err := t.dial(ctx)
		if err != nil {
			t.logger.Warn("websocket: reconnect dial failed", "err", err)
			continue
		}
		t.logger.Info("websocket: reconnected")
		return conn
	}
}

// handleMessage processes a single inbound server message.
func (t *WebSocketTransport) handleMessage(_ context.Context, _ *websocket.Conn, msg *wsMessage) {
	switch msg.Type {
	case "flags":
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
	default:
		t.logger.Debug("websocket: unknown message type", "type", msg.Type)
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

// Ensure WebSocketTransport satisfies the Transport interface at compile time.
var _ Transport = (*WebSocketTransport)(nil)
