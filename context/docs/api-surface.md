# API Surface Context

This repository exposes an internal platform-to-engine API, not a public user API.

Flow:

```txt
user -> trading platform -> matching engine
```

The trading platform owns authentication, account ownership, balance checks, reservation, public rate limits, and user-facing error messages. The matching engine only processes validated order commands and returns engine-level command/query results.

In scope:

- Command-oriented order API:
  - `POST /commands/orders/create` creates an order command.
  - `POST /commands/orders/amend` amends an active order.
  - `POST /commands/orders/cancel` cancels an active order; it does not erase history.
- Query API:
  - `GET /queries/orderbook?ticker={ticker}&depth=N` returns aggregated price levels from an orderbook snapshot.
- Readiness API:
  - `GET /ready` returns `200` only while the engine accepts commands and
    returns `503` after persistence failure or shutdown begins.

Out of scope:

- Public user API semantics.
- Authentication, balances, custody, settlement, and account authorization.
- Raw match log query endpoints.
- Trade history, candles, volume, analytics, or ETL APIs.
- WebSocket delta streams in the first API implementation.
- Extra API layers or specs until needed.

Design rules:

- Do not expose `OrderBook`, heap, queue, or worker internals directly.
- HTTP handlers should send commands/queries through engine-owned boundaries.
- Order endpoints enqueue commands; matching remains inside `BookWorker`.
- Orderbook reads must use a worker-owned snapshot path, not direct concurrent reads of `worker.OrderBook`.
- Keep response DTOs small and engine-facing. Platform translates them into public user responses.

Implementation shape:

- Use the standard `net/http` server and method-aware `http.ServeMux`; this API does not need a framework dependency.
- Keep HTTP transport code in `internal/api`, separate from the runnable wiring in `cmd/server` and engine behavior in `internal/engine`.
- Handlers decode and validate requests, map engine errors to HTTP responses, and dispatch commands or queries through `Router`.
- Route orderbook snapshots through the same worker event queue as commands so each ticker preserves command/query ordering.
- Map fail-closed engine state to `503 Service Unavailable` for commands and readiness.
- Accept limit order commands only until market-order matching and residual behavior are defined in the engine.

Verify:

- Documentation-only API surface changes need path and responsibility review.
- Endpoint implementations must add focused handler/engine tests and pass `go test ./...`.
