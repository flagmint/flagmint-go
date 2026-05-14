package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/flagmint/flagmint-go/internal/backoff"
)

const (
	evaluatePath    = "/evaluator/evaluate"
	defaultPollInterval = 20 * time.Minute
)

// evaluateRequest is the JSON body sent to the evaluate endpoint.
type evaluateRequest struct {
	Context map[string]any `json:"context"`
}

// HTTPTransport polls the Flagmint backend periodically over HTTPS (long-polling).
// Safe for concurrent use after Connect() returns.
type HTTPTransport struct {
	endpoint     string
	apiKey       string
	logger       *slog.Logger
	httpClient   *http.Client
	pollInterval time.Duration

	// cbMu guards onUpdated.
	cbMu      sync.Mutex
	onUpdated func(map[string]any)

	// mu guards lastEvalCtx and lastFlags.
	mu          sync.Mutex
	lastEvalCtx map[string]any
	lastFlags   map[string]any

	// Lifecycle
	innerCtx    context.Context
	innerCancel context.CancelFunc
	closeOnce   sync.Once
	wg          sync.WaitGroup
}

// HTTPTransportOptions configures optional parameters for HTTPTransport.
type HTTPTransportOptions struct {
	// PollInterval controls how often the transport polls the evaluate endpoint.
	// Defaults to 20 minutes when zero.
	PollInterval time.Duration
	// HTTPClient overrides the default HTTP client. Useful for testing.
	HTTPClient *http.Client
}

// NewHTTPTransport creates a new HTTPTransport with default poll interval (20 minutes).
func NewHTTPTransport(endpoint, apiKey string, logger *slog.Logger) *HTTPTransport {
	return NewHTTPTransportWithOptions(endpoint, apiKey, logger, HTTPTransportOptions{})
}

// NewHTTPTransportWithOptions creates a new HTTPTransport with explicit options.
func NewHTTPTransportWithOptions(endpoint, apiKey string, logger *slog.Logger, opts HTTPTransportOptions) *HTTPTransport {
	interval := opts.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPTransport{
		endpoint:     endpoint,
		apiKey:       apiKey,
		logger:       logger,
		httpClient:   hc,
		pollInterval: interval,
	}
}

// OnFlagsUpdated registers the callback invoked when the server pushes flag updates.
// Must be called before Connect.
func (t *HTTPTransport) OnFlagsUpdated(fn func(flags map[string]any)) {
	t.cbMu.Lock()
	defer t.cbMu.Unlock()
	t.onUpdated = fn
}

// Connect makes an initial flag fetch attempt and starts the background poll loop.
// If the initial fetch fails (e.g. server unreachable), Connect logs a warning
// and starts the poll loop anyway; retries will happen at the configured interval.
// Connect blocks until the poll loop is started or ctx is cancelled.
func (t *HTTPTransport) Connect(ctx context.Context) error {
	inner, cancel := context.WithCancel(context.Background())
	t.innerCtx = inner
	t.innerCancel = cancel

	// Attempt an initial fetch. Failure is non-fatal — the poll loop will retry.
	flags, err := t.postEvaluate(ctx, nil)
	if err != nil {
		if ctx.Err() != nil {
			cancel()
			return ctx.Err()
		}
		t.logger.Warn("http transport: initial fetch failed, will retry during polling", "err", err)
	} else {
		t.mu.Lock()
		t.lastFlags = flags
		t.mu.Unlock()
		t.notifyUpdated(flags)
	}

	t.wg.Add(1)
	go t.pollLoop(inner)

	t.logger.Info("http transport: connected", "endpoint", t.endpoint)
	return nil
}

// FetchFlags sends the evaluation context to the evaluate endpoint and returns
// the flag set.
func (t *HTTPTransport) FetchFlags(ctx context.Context, evalCtx map[string]any) (map[string]any, error) {
	t.mu.Lock()
	t.lastEvalCtx = evalCtx
	t.mu.Unlock()

	return t.postEvaluate(ctx, evalCtx)
}

// Close stops the poll loop and waits for all goroutines to exit.
// Safe to call multiple times.
func (t *HTTPTransport) Close() error {
	t.closeOnce.Do(func() {
		if t.innerCancel != nil {
			t.innerCancel()
		}
	})
	t.wg.Wait()
	return nil
}

// pollLoop runs in a goroutine and polls the evaluate endpoint every pollInterval.
func (t *HTTPTransport) pollLoop(ctx context.Context) {
	defer t.wg.Done()

	bo := &backoff.Backoff{
		Base:       5 * time.Second,
		Multiplier: 2.0,
		MaxDelay:   60 * time.Second,
		Jitter:     0.2,
	}

	timer := time.NewTimer(t.pollInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		t.mu.Lock()
		evalCtx := t.lastEvalCtx
		t.mu.Unlock()

		flags, err := t.postEvaluate(ctx, evalCtx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			t.logger.Warn("http transport: poll error", "err", err)
			delay := bo.Next()
			timer.Reset(delay)
			continue
		}

		bo.Reset()

		// Notify only if flags changed.
		t.mu.Lock()
		changed := !mapsEqual(t.lastFlags, flags)
		if changed {
			t.lastFlags = flags
		}
		t.mu.Unlock()

		if changed {
			t.notifyUpdated(flags)
		}

		timer.Reset(t.pollInterval)
	}
}

// postEvaluate makes a POST request to the evaluate endpoint and returns the flags.
// evalCtx may be nil if no context is available yet.
func (t *HTTPTransport) postEvaluate(ctx context.Context, evalCtx map[string]any) (map[string]any, error) {
	body := evaluateRequest{Context: evalCtx}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := t.endpoint + evaluatePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", t.apiKey)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var flags map[string]any
	if err := json.Unmarshal(respData, &flags); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return flags, nil
}

// notifyUpdated calls the registered callback with the given flags.
func (t *HTTPTransport) notifyUpdated(flags map[string]any) {
	t.cbMu.Lock()
	fn := t.onUpdated
	t.cbMu.Unlock()
	if fn != nil {
		fn(flags)
	}
}

// mapsEqual reports whether two flag maps have equal contents using deep equality.
func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	// Use JSON round-trip for deep comparison of nested values, which handles
	// different numeric types consistently (all numbers come back as float64).
	aJSON, errA := json.Marshal(a)
	bJSON, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		// On marshal failure, fall back to length-only comparison (already done above).
		return false
	}
	return string(aJSON) == string(bJSON)
}

// Ensure HTTPTransport satisfies the Transport interface at compile time.
var _ Transport = (*HTTPTransport)(nil)
