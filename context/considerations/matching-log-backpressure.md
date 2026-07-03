# Matching Log Backpressure Considerations

Status: open consideration

This note records current tradeoffs for raw match log delivery. It is not a
final durability policy.

## Current Direction

- `BookWorker` should not own a `matchlog.Store` or call DB writes directly.
- `BookWorker` may emit `MatchResult.Logs` to a match log output channel.
- A separate match log writer should own `matchlog.Store` and persistence.
- Raw match logs are source-of-truth execution records, not analytics trades.

## Conservative Initial Policy

- Use a bounded channel as a short burst buffer.
- Default channel slot counts are configurable in `config/default.json`.
- Block when the channel is full instead of dropping logs.
- Treat the channel as an in-process handoff, not durable storage.
- Use shared DB connection pooling; do not dedicate one DB connection per writer.

This favors raw log integrity over ticker-level matching availability. If the
writer or database stalls, the affected matching path may backpressure.

## Why Not Drop Or Buffer Forever

- Dropping logs can leave executed matches without source records.
- Unbounded in-memory buffering can hide DB failure until the process runs out
  of memory.
- An in-memory database is not a safe source of truth for executed matches.

## Deferred Decisions

- Exact match log output channel buffer size.
- Batch insert strategy.
- Retry and timeout behavior.
- Shutdown drain behavior.
- Durable commit log, local WAL, Kafka, Redpanda, or NATS JetStream.
- Whether DB failure should halt one ticker, all tickers, or trigger another
  operational mode.

## Promotion Rule

When one of these policies is finalized, move the conclusion into a decision
record and keep this file as background context.
