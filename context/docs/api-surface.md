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

Verify:

- Documentation-only API surface changes need path and responsibility review.
- Endpoint implementations must add focused handler/engine tests and pass `go test ./...`.
