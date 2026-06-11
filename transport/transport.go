// Package transport defines the Transport interface and its implementations.
package transport

import "context"

// Transport abstracts the communication layer between the SDK and Flagmint servers.
// Implementations must be safe for concurrent use after Connect() returns.
type Transport interface {
	// Connect establishes the connection with optional initial evaluation context.
	// The context is sent in the x-flagmint-context header if provided.
	// Blocks until the transport is ready to send/receive, or ctx is cancelled.
	Connect(ctx context.Context, evalCtx map[string]any) error

	// FetchFlags sends the evaluation context to the server and returns the
	// evaluated flag set. evalCtx is a map representation of the evaluation
	// context. For WebSocket this sends a context message; for HTTP this makes
	// a POST request.
	FetchFlags(ctx context.Context, evalCtx map[string]any) (map[string]any, error)

	// OnFlagsUpdated registers a callback invoked when the server pushes flag
	// updates (WebSocket broadcasts or poll results with changes).
	// Must be called before Connect(). Only one callback is supported;
	// subsequent calls replace the previous callback.
	OnFlagsUpdated(fn func(flags map[string]any))

	// Close shuts down the transport and releases resources.
	// Blocks until all internal goroutines have exited.
	// Safe to call multiple times.
	Close() error
}
