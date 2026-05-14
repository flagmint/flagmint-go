package transport_test

import (
	"context"
	"testing"
	"time"

	"github.com/flagmint/flagmint-go/transport"
	"go.uber.org/goleak"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// TestAutoTransport_FallbackToHTTP verifies that when the WebSocket endpoint
// is unreachable, the AutoTransport transparently falls back to HTTP.
func TestAutoTransport_FallbackToHTTP(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Start a mock HTTP server but no WebSocket server.
	httpSrv := newMockHTTPServer(t, flagHandler(map[string]any{"fallback": true}))
	defer httpSrv.Close()

	flagsCh := make(chan map[string]any, 1)

	// Point WebSocket at an unreachable address (closed port).
	const unreachableWS = "ws://127.0.0.1:1" // port 1 is never open
	tr := transport.NewAutoTransport(unreachableWS, httpSrv.URL, "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(flags map[string]any) {
		select {
		case flagsCh <- flags:
		default:
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect (auto fallback): %v", err)
	}
	defer tr.Close() //nolint:errcheck

	select {
	case flags := <-flagsCh:
		if v, ok := flags["fallback"]; !ok || v != true {
			t.Errorf("fallback flag: got %v, want true", v)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for flags via HTTP fallback")
	}
}

// TestAutoTransport_WebSocket verifies that when the WebSocket endpoint is
// reachable, the AutoTransport uses it.
func TestAutoTransport_WebSocket(t *testing.T) {
	defer goleak.VerifyNone(t)

	flagsCh := make(chan map[string]any, 1)

	wsSrv := newMockWSServer(t, func(conn *websocket.Conn) {
		defer conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
		_ = wsjson.Write(context.Background(), conn, wsMessage{
			Type:  "flags",
			Flags: map[string]any{"ws-flag": true},
		})
		var dummy wsMessage
		_ = wsjson.Read(context.Background(), conn, &dummy)
	})
	defer wsSrv.Close()

	wsEndpoint := "ws" + wsSrv.URL[4:]
	// HTTP endpoint is unreachable — should not be used.
	tr := transport.NewAutoTransport(wsEndpoint, "http://127.0.0.1:1", "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(flags map[string]any) {
		select {
		case flagsCh <- flags:
		default:
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	select {
	case flags := <-flagsCh:
		if v, ok := flags["ws-flag"]; !ok || v != true {
			t.Errorf("ws-flag: got %v, want true", v)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for flags via WebSocket")
	}
}

// TestAutoTransport_Close_Idempotent verifies Close can be called multiple times.
func TestAutoTransport_Close_Idempotent(t *testing.T) {
	defer goleak.VerifyNone(t)

	httpSrv := newMockHTTPServer(t, flagHandler(map[string]any{}))
	defer httpSrv.Close()

	const unreachableWS = "ws://127.0.0.1:1"
	tr := transport.NewAutoTransport(unreachableWS, httpSrv.URL, "k", silentLogger(t))
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestAutoTransport_FetchFlags_Fallback verifies FetchFlags works when using
// the HTTP fallback transport.
func TestAutoTransport_FetchFlags_Fallback(t *testing.T) {
	defer goleak.VerifyNone(t)

	responseFlags := map[string]any{"fetched": true}
	httpSrv := newMockHTTPServer(t, flagHandler(responseFlags))
	defer httpSrv.Close()

	const unreachableWS = "ws://127.0.0.1:1"
	tr := transport.NewAutoTransport(unreachableWS, httpSrv.URL, "k", silentLogger(t))
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	flags, err := tr.FetchFlags(ctx, map[string]any{"kind": "user", "key": "u1"})
	if err != nil {
		t.Fatalf("FetchFlags: %v", err)
	}
	if v, ok := flags["fetched"]; !ok || v != true {
		t.Errorf("fetched: got %v, want true", v)
	}
}
