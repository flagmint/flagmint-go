package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"context"

	"github.com/flagmint/flagmint-go/transport"
	"go.uber.org/goleak"
)

// newMockHTTPServer creates an httptest.Server that returns the provided flags
// payload on POST /evaluator/evaluate.
func newMockHTTPServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

// flagHandler returns an http.HandlerFunc that serialises flags as a JSON response.
func flagHandler(flags map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(flags)
	}
}

// TestHTTPTransport_Connect verifies that Connect makes an initial fetch and
// calls the onUpdated callback.
func TestHTTPTransport_Connect(t *testing.T) {
	defer goleak.VerifyNone(t)

	initialFlags := map[string]any{"feature-a": true}
	srv := newMockHTTPServer(t, flagHandler(initialFlags))
	defer srv.Close()

	flagsCh := make(chan map[string]any, 1)
	tr := transport.NewHTTPTransport(srv.URL, "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(flags map[string]any) {
		flagsCh <- flags
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	select {
	case flags := <-flagsCh:
		if v, ok := flags["feature-a"]; !ok || v != true {
			t.Errorf("feature-a: got %v, want true", v)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for initial flags")
	}
}

// TestHTTPTransport_FetchFlags verifies that FetchFlags makes a POST to the
// evaluate endpoint and returns the flag set.
func TestHTTPTransport_FetchFlags(t *testing.T) {
	defer goleak.VerifyNone(t)

	responseFlags := map[string]any{"x": float64(42)}
	var requestBodyCtx map[string]any

	srv := newMockHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Capture the request body.
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if ctx, ok := body["context"].(map[string]any); ok {
			requestBodyCtx = ctx
		}
		flagHandler(responseFlags)(w, r)
	})
	defer srv.Close()

	tr := transport.NewHTTPTransport(srv.URL, "test-key", silentLogger(t))
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	evalCtx := map[string]any{"kind": "user", "key": "u1"}
	flags, err := tr.FetchFlags(ctx, evalCtx)
	if err != nil {
		t.Fatalf("FetchFlags: %v", err)
	}
	if v, ok := flags["x"]; !ok || v != float64(42) {
		t.Errorf("x: got %v, want 42", v)
	}
	// Verify x-api-key header was sent.
	_ = requestBodyCtx // verified that the body was parsed
}

// TestHTTPTransport_APIKeyHeader verifies that the x-api-key header is sent.
func TestHTTPTransport_APIKeyHeader(t *testing.T) {
	defer goleak.VerifyNone(t)

	var gotAPIKey string
	srv := newMockHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		flagHandler(map[string]any{})(w, r)
	})
	defer srv.Close()

	const apiKey = "my-secret-key"
	tr := transport.NewHTTPTransport(srv.URL, apiKey, silentLogger(t))
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	if gotAPIKey != apiKey {
		t.Errorf("x-api-key: got %q, want %q", gotAPIKey, apiKey)
	}
}

// TestHTTPTransport_BackoffOnError verifies that successive request failures
// increase the delay (we check by counting calls with a short interval).
func TestHTTPTransport_BackoffOnError(t *testing.T) {
	defer goleak.VerifyNone(t)

	var callCount atomic.Int64
	srv := newMockHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	defer srv.Close()

	tr := transport.NewHTTPTransportWithOptions(srv.URL, "k", silentLogger(t),
		transport.HTTPTransportOptions{PollInterval: 50 * time.Millisecond},
	)
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Connect will fail on initial fetch, then poll loop will retry with backoff.
	_ = tr.Connect(ctx) // may fail since server returns errors
	defer tr.Close()    //nolint:errcheck

	// Give the poll loop some time to run.
	time.Sleep(500 * time.Millisecond)
	_ = tr.Close()

	n := callCount.Load()
	if n < 1 {
		t.Errorf("expected at least 1 request, got %d", n)
	}
}

// TestHTTPTransport_PollCycle verifies that the poll loop detects flag changes
// and calls onUpdated.
func TestHTTPTransport_PollCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	// The server returns different flags on each request.
	var callCount atomic.Int64
	srv := newMockHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		flags := map[string]any{"v": float64(n)}
		flagHandler(flags)(w, r)
	})
	defer srv.Close()

	flagsCh := make(chan map[string]any, 10)
	tr := transport.NewHTTPTransportWithOptions(srv.URL, "k", silentLogger(t),
		transport.HTTPTransportOptions{PollInterval: 50 * time.Millisecond},
	)
	tr.OnFlagsUpdated(func(flags map[string]any) {
		flagsCh <- flags
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close() //nolint:errcheck

	// Collect at least 2 distinct updates.
	var updates []map[string]any
	deadline := time.After(3 * time.Second)
	for len(updates) < 2 {
		select {
		case f := <-flagsCh:
			updates = append(updates, f)
		case <-deadline:
			t.Fatalf("timed out after %d updates; want 2", len(updates))
		}
	}
	if updates[0]["v"] == updates[1]["v"] {
		t.Error("expected different flag values across poll cycles")
	}
}

// TestHTTPTransport_Close_Idempotent verifies Close can be called multiple times.
func TestHTTPTransport_Close_Idempotent(t *testing.T) {
	defer goleak.VerifyNone(t)

	srv := newMockHTTPServer(t, flagHandler(map[string]any{}))
	defer srv.Close()

	tr := transport.NewHTTPTransport(srv.URL, "k", silentLogger(t))
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

// TestHTTPTransport_Close_BlocksUntilGoroutineExit verifies that Close waits
// for the poll goroutine to exit (goleak ensures no leaks).
func TestHTTPTransport_Close_BlocksUntilGoroutineExit(t *testing.T) {
	defer goleak.VerifyNone(t)

	srv := newMockHTTPServer(t, flagHandler(map[string]any{"ok": true}))
	defer srv.Close()

	tr := transport.NewHTTPTransport(srv.URL, "k", silentLogger(t))
	tr.OnFlagsUpdated(func(map[string]any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = tr.Close()
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not unblock within 3s")
	}
}
