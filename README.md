# Flagmint Go SDK

The official Go SDK for [Flagmint](https://docs.flagmint.com/sdks/go) feature flags.

[![Go Reference](https://pkg.go.dev/badge/github.com/flagmint/flagmint-go.svg)](https://pkg.go.dev/github.com/flagmint/flagmint-go)

---

## Requirements

- Go 1.21 or later

## Installation

```bash
go get github.com/flagmint/flagmint-go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    flagmint "github.com/flagmint/flagmint-go"
)

func main() {
    client, err := flagmint.NewClient("fm_sdk_your_api_key",
        flagmint.WithContext(flagmint.EvaluationContext{
            Kind: "user",
            Key:  "user-123",
            Attributes: map[string]any{
                "country": "DE",
                "plan":    "pro",
            },
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := client.Ready(ctx); err != nil {
        log.Fatal("failed to initialise:", err)
    }

    if client.BoolFlag("dark-mode", false) {
        fmt.Println("Dark mode is enabled!")
    }

    // Subscribe to flag changes
    unsub := client.Subscribe(func(flags flagmint.FeatureFlags) {
        fmt.Println("Flags updated:", flags.Len(), "flags")
    })
    defer unsub()
}
```

## Transport Modes

The SDK supports three transport modes, controlled by `WithTransportMode`:

| Mode | Constant | When to use |
|------|----------|-------------|
| **Auto** (default) | `flagmint.TransportAuto` | Tries WebSocket first, falls back to HTTP long-polling automatically |
| **WebSocket** | `flagmint.TransportWebSocket` | When you want persistent, low-latency streaming and your environment supports WebSocket |
| **HTTP long-polling** | `flagmint.TransportLongPolling` | Environments that block WebSocket connections (some proxies, serverless) |

```go
// Force WebSocket
client, _ := flagmint.NewClient(apiKey,
    flagmint.WithTransportMode(flagmint.TransportWebSocket),
)

// Force HTTP long-polling
client, _ := flagmint.NewClient(apiKey,
    flagmint.WithTransportMode(flagmint.TransportLongPolling),
)
```

The default `TransportAuto` mode attempts a WebSocket connection and transparently falls back to HTTP if the connection cannot be established.

## Local Evaluation

By default, flag evaluation is performed server-side (remote evaluation). With `WithLocalEvaluation()`, the SDK downloads the full flag rule configuration and evaluates flags locally — no network round-trip per evaluation.

```go
client, err := flagmint.NewClient("fm_sdk_your_api_key",
    flagmint.WithLocalEvaluation(),
    flagmint.WithContext(flagmint.EvaluationContext{
        Kind: "user",
        Key:  "user-456",
        Attributes: map[string]any{
            "country": "FR",
            "plan":    "growth",
        },
    }),
)
// ... Ready(), then:

// Evaluation happens locally — no network call per flag check
enabled := client.BoolFlag("checkout-redesign", false)
```

Supply flag configurations via `SetFlagConfigs`. In production, fetch these from the `/evaluator/config` endpoint and call `SetFlagConfigs` whenever the config changes.

### Remote vs Local Evaluation

| Aspect | Remote | Local |
|--------|--------|-------|
| Latency | 5–50 ms per flag | < 0.1 ms per flag |
| Network dependency | Every evaluation | Config fetch only |
| Billing | Per evaluation call | Per config fetch |
| Flag config visibility | Server-side only | Full config on client |
| Best for | Client-side SDKs, low-volume | Server-side SDKs, high-volume |

## Evaluation Context

The `EvaluationContext` tells Flagmint who is requesting the flag evaluation. It is used for targeting rules and percentage rollouts.

```go
// Single user context
ctx := flagmint.EvaluationContext{
    Kind: "user",
    Key:  "user-123",
    Attributes: map[string]any{
        "country": "DE",
        "plan":    "pro",
        "email":   "alice@example.com",
    },
}

// Organization context
ctx := flagmint.EvaluationContext{
    Kind: "organization",
    Key:  "org-456",
    Attributes: map[string]any{
        "plan": "enterprise",
    },
}

// Multi-context (user + organization)
ctx := flagmint.EvaluationContext{
    Kind: "multi",
    User: &flagmint.ContextEntity{
        Key: "user-123",
        Attributes: map[string]any{"plan": "pro"},
    },
    Organization: &flagmint.ContextEntity{
        Key: "org-456",
        Attributes: map[string]any{"plan": "enterprise"},
    },
}
```

Update the context at runtime with `UpdateContext`:

```go
client.UpdateContext(flagmint.EvaluationContext{
    Kind: "user",
    Key:  "user-789",
})
```

## Caching

The SDK includes an in-memory cache enabled by default when `WithCache(true)` is passed. Cached flags are served immediately on startup (degraded-mode support) while the transport reconnects.

```go
// Enable built-in in-memory cache (24-hour TTL)
client, _ := flagmint.NewClient(apiKey, flagmint.WithCache(true))
```

### Custom Cache Adapters

Implement the `CacheAdapter` interface to use an external store (e.g., Redis):

```go
type CacheAdapter interface {
    LoadFlags(apiKey string) (*FeatureFlags, error)
    SaveFlags(apiKey string, flags FeatureFlags) error
    LoadContext(apiKey string) (*EvaluationContext, error)
    SaveContext(apiKey string, ctx *EvaluationContext) error
}

// Plug in a custom adapter
client, _ := flagmint.NewClient(apiKey,
    flagmint.WithCacheAdapter(myRedisAdapter),
)
```

See [`examples/custom-cache/`](./examples/custom-cache) for a Redis-backed example.

## Thread Safety

`FlagClient` is safe for concurrent use by multiple goroutines. All public methods (`BoolFlag`, `StringFlag`, `GetFlags`, `UpdateContext`, `Subscribe`, etc.) may be called from any goroutine without additional locking.

## Configuration Options

| Option | Description |
|--------|-------------|
| `WithContext(ctx)` | Set the default evaluation context |
| `WithTransportMode(mode)` | Choose `auto` (default), `websocket`, or `long-polling` |
| `WithLocalEvaluation()` | Enable local flag evaluation (no per-flag network calls) |
| `WithCache(enabled)` | Enable/disable the built-in in-memory flag cache |
| `WithCacheAdapter(adapter)` | Plug in a custom cache (e.g., Redis) |
| `WithOnError(fn)` | Register a non-fatal error callback |
| `WithEndpoints(rest, ws)` | Override the default API endpoints |
| `WithDeferInit()` | Delay connecting until `Initialize()` is called |
| `WithLogger(l)` | Supply a custom `*slog.Logger` |

## Examples

See the [`examples/`](./examples) directory for runnable examples:

- [`examples/basic/`](./examples/basic) — Remote evaluation, simplest possible usage
- [`examples/local-eval/`](./examples/local-eval) — Local evaluation with targeting rules
- [`examples/gin-middleware/`](./examples/gin-middleware) — Feature flag middleware for Gin
- [`examples/custom-cache/`](./examples/custom-cache) — Redis-backed cache adapter

## Full Documentation

Full documentation is available at [docs.flagmint.com](https://docs.flagmint.com).

## Development

```bash
# Build
go build ./...

# Test
go test ./...

# Vet
go vet ./...
```

## License

BSD-3-Clause — see [LICENSE](./LICENSE) for details.
