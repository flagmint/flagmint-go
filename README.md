# Flagmint Go SDK

The official Go SDK for [Flagmint](https://flagmint.com) feature flags.

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
    "fmt"
    "log"

    flagmint "github.com/flagmint/flagmint-go"
)

func main() {
    client, err := flagmint.NewClient("your-api-key",
        flagmint.WithContext(flagmint.EvaluationContext{
            Kind: "user",
            Key:  "user-123",
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    if val, ok := client.GetFlag("my-feature"); ok {
        fmt.Printf("my-feature = %v\n", val)
    }
}
```

## Configuration Options

| Option | Description |
|---|---|
| `WithContext(ctx)` | Set the default evaluation context |
| `WithTransportMode(mode)` | Choose `auto` (default), `websocket`, or `long-polling` |
| `WithCache(enabled)` | Enable/disable the in-memory flag cache |
| `WithCacheAdapter(adapter)` | Plug in a custom cache (e.g., Redis) |
| `WithOnError(fn)` | Register a non-fatal error callback |
| `WithEndpoints(rest, ws)` | Override the default API endpoints |
| `WithDeferInit()` | Delay connecting until `Initialize()` is called |
| `WithLogger(l)` | Supply a custom `*slog.Logger` |

## Examples

See the [`examples/`](./examples) directory for runnable examples:

- [`examples/basic/`](./examples/basic) — Basic flag retrieval
- [`examples/local-eval/`](./examples/local-eval) — Offline/local evaluation

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
