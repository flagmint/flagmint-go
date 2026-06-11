# Changelog

All notable changes to the Flagmint Go SDK will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note:** This SDK is pre-stable. Breaking changes may occur before `v1.0.0`.

---

## [v0.1.0] — 2026-05-31

### Added

- **Core client** (`FlagClient`) with `NewClient`, `Ready`, `Close`, `BoolFlag`,
  `StringFlag`, `NumberFlag`, `JSONFlag`, `GetFlag`, `GetFlags`, `UpdateContext`,
  and `Subscribe` APIs.
- **Transport layer** with three modes:
  - `TransportAuto` (default) — WebSocket preferred, falls back to HTTP long-polling.
  - `TransportWebSocket` — persistent WebSocket connection (`/ws/sdk`).
  - `TransportLongPolling` — HTTP long-polling (`/evaluator/evaluate`).
- **Local evaluation** via `WithLocalEvaluation()` and `SetFlagConfigs`. The
  `evaluate` sub-package contains the rule engine (`Evaluator`), targeting rule
  types, and rollout bucketing using the djb2 string-hash algorithm.
- **Caching** via the `CacheAdapter` interface. Built-in in-memory cache with a
  24-hour TTL; `NopCache` provided for testing. `FeatureFlags` implements
  `json.Marshaler`/`json.Unmarshaler` for easy serialisation in custom adapters.
- **Typed convenience methods** `BoolFlag`, `StringFlag`, `NumberFlag` route
  through the local evaluator when `WithLocalEvaluation()` is active.
- **Subscription** system: `Subscribe(fn)` fires immediately with the current
  flag state and on every subsequent update; returns an unsubscribe function.
- **Configuration options**: `WithContext`, `WithTransportMode`,
  `WithLocalEvaluation`, `WithCache`, `WithCacheAdapter`, `WithOnError`,
  `WithEndpoints`, `WithDeferInit`, `WithLogger`.
- **Endpoint resolution** from environment variables (`FLAGMINT_REST_ENDPOINT`,
  `FLAGMINT_WS_ENDPOINT`, `FLAGMINT_ENV`).
- **Examples**:
  - `examples/basic/` — remote evaluation, simplest usage.
  - `examples/local-eval/` — local evaluation with targeting rules.
  - `examples/gin-middleware/` — per-request feature flag middleware for Gin.
  - `examples/custom-cache/` — Redis-backed `CacheAdapter` implementation.
- **GoDoc** package-level doc comment and `Example_` functions for `NewClient`,
  `BoolFlag`, `Subscribe`, and `WithLocalEvaluation`.
- **Mintlify documentation** under `docs/sdk/go/`:
  - `quickstart.mdx`
  - `configuration.mdx`
  - `local-evaluation.mdx`
  - `transport.mdx`
  - `caching.mdx`

[v0.1.0]: https://github.com/flagmint/flagmint-go/releases/tag/v0.1.0
