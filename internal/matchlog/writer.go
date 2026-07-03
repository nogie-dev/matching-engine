package matchlog

import (
	"context"
	"log/slog"
)

type Writer struct {
	store Store
}

func NewWriter(store Store) *Writer {
	return &Writer{store: store}
}

func (w *Writer) Run(ctx context.Context, in <-chan []MatchLog) {
	for {
		select {
		case <-ctx.Done():
			return
		case logs, ok := <-in:
			if !ok {
				return
			}
			if len(logs) == 0 || w.store == nil {
				continue
			}
			if err := w.store.SaveMatchLogs(ctx, logs); err != nil {
				// ponytail: durability policy is unresolved; decide retry/backpressure/drop before production use.
				slog.Error("match log store failed", "error", err)
			}
		}
	}
}
