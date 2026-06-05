package transport_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/flagmint/flagmint-go/transport"
	"go.uber.org/goleak"
)

// wsMessage mirrors the wire format used by the WebSocket transport.
type wsMessage struct {
	Type    string         `json:"type"`
	Context map[string]any `json:"context,omitempty"`
	Flags   map[string]any `json:"flags,omitempty"`
	Key     string         `json:"key,omitempty"`
	Value   any            `json:"value,omitempty"`
}

// newMockWSServer creates an httptest server that accepts WebSocket connections
// and runs the provided handler in a goroutine for each connection.
func newMockWSServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Logf("mock ws server: accept error: %v", err)
			return
		}
		handler(conn)
	}))
	return srv
}

// TestWebSocketTransport_ConnectAndReceiveFlags verifies that the transport
// connects to a mock WS server and delivers server-pushed flag updates.
func TestWebSocketTransport_ConnectAndReceiveFlags(t *testing.T) {
	defer goleak.VerifyNone(t)

	flagsCh := make(chan map[string]any, 1)

	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		defer conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
		// Push a flags broadcast as soon as the client connects.
		msg := wsMessage{
			Type:  "flags",
			Flags: map[string]any{"dark-mode": true, "beta": false},
		}
		_ = wsjson.Write(context.Background(), conn, msg)
		// Keep alive until connection is closed.
		var dummy wsMessage
		_ = wsjson.Read(context.Background(), conn, &dummy)
	})
	defer srv.Close()

	wsEndpoint := "ws" + srv.URL[4:] // http → ws
	tr := transport.NewWebSocketTransport(wsEndpoint, "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(flags map[string]any) {
		flagsCh <- flags
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx, map[string]any{}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	select {
	case flags := <-flagsCh:
		if v, ok := flags["dark-mode"]; !ok || v != true {
			t.Errorf("dark-mode flag: got %v, want true", v)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for flags update")
	}
}

// TestWebSocketTransport_FetchFlags verifies that FetchFlags sends a context
// message and receives the flag response.
func TestWebSocketTransport_FetchFlags(t *testing.T) {
	defer goleak.VerifyNone(t)

	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		defer conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
		// Read the context message, then reply with flags.
		var req wsMessage
		if err := wsjson.Read(context.Background(), conn, &req); err != nil {
			t.Logf("mock: read error: %v", err)
			return
		}
		reply := wsMessage{
			Type:  "flags",
			Flags: map[string]any{"feature-x": true},
		}
		_ = wsjson.Write(context.Background(), conn, reply)
		// Drain so the connection stays open.
		var dummy wsMessage
		_ = wsjson.Read(context.Background(), conn, &dummy)
	})
	defer srv.Close()

	wsEndpoint := "ws" + srv.URL[4:]
	tr := transport.NewWebSocketTransport(wsEndpoint, "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx, map[string]any{}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	evalCtx := map[string]any{"kind": "user", "key": "u123"}
	flags, err := tr.FetchFlags(ctx, evalCtx)
	if err != nil {
		t.Fatalf("FetchFlags: %v", err)
	}
	if v, ok := flags["feature-x"]; !ok || v != true {
		t.Errorf("feature-x: got %v, want true", v)
	}
}

// TestWebSocketTransport_ContextCancellation verifies that cancelling ctx
// during/after Connect causes the readLoop to exit cleanly.
func TestWebSocketTransport_ContextCancellation(t *testing.T) {
	defer goleak.VerifyNone(t)

	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		defer conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
		// Block until the connection is closed.
		var dummy wsMessage
		_ = wsjson.Read(context.Background(), conn, &dummy)
	})
	defer srv.Close()

	wsEndpoint := "ws" + srv.URL[4:]
	tr := transport.NewWebSocketTransport(wsEndpoint, "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx, map[string]any{}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Close immediately — goroutines should exit.
	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestWebSocketTransport_Close_Idempotent verifies Close can be called multiple times.
func TestWebSocketTransport_Close_Idempotent(t *testing.T) {
	defer goleak.VerifyNone(t)

	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		defer conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
		var dummy wsMessage
		_ = wsjson.Read(context.Background(), conn, &dummy)
	})
	defer srv.Close()

	wsEndpoint := "ws" + srv.URL[4:]
	tr := transport.NewWebSocketTransport(wsEndpoint, "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx, map[string]any{}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestWebSocketTransport_Reconnect verifies that the transport reconnects after
// the server drops the connection.
func TestWebSocketTransport_Reconnect(t *testing.T) {
	defer goleak.VerifyNone(t)

	var connCount int
	var mu sync.Mutex
	flagsCh := make(chan map[string]any, 2)

	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		mu.Lock()
		connCount++
		n := connCount
		mu.Unlock()

		defer conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck

		if n == 1 {
			// First connection: send flags then immediately close to trigger reconnect.
			_ = wsjson.Write(context.Background(), conn, wsMessage{
				Type:  "flags",
				Flags: map[string]any{"v": float64(1)},
			})
			return // closing immediately forces reconnect
		}
		// Second connection: send updated flags.
		_ = wsjson.Write(context.Background(), conn, wsMessage{
			Type:  "flags",
			Flags: map[string]any{"v": float64(2)},
		})
		// Keep alive.
		var dummy wsMessage
		_ = wsjson.Read(context.Background(), conn, &dummy)
	})
	defer srv.Close()

	wsEndpoint := "ws" + srv.URL[4:]
	tr := transport.NewWebSocketTransport(wsEndpoint, "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(flags map[string]any) {
		flagsCh <- flags
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := tr.Connect(ctx, map[string]any{}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	// Collect two flag updates: one from first connection, one from reconnect.
	var updates []map[string]any
	for len(updates) < 2 {
		select {
		case f := <-flagsCh:
			updates = append(updates, f)
		case <-ctx.Done():
			t.Fatalf("timed out after %d updates; want 2", len(updates))
		}
	}

	if v, ok := updates[1]["v"]; !ok || v != float64(2) {
		t.Errorf("after reconnect v = %v, want 2", v)
	}
}

// silentLogger returns a *slog.Logger that discards all output during tests.
func silentLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelError}))
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
