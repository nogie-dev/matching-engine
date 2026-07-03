package matchlog

import (
	"context"
	"testing"
)

type fakeStore struct {
	logs []MatchLog
}

func (f *fakeStore) SaveMatchLog(ctx context.Context, log MatchLog) error {
	return f.SaveMatchLogs(ctx, []MatchLog{log})
}

func (f *fakeStore) SaveMatchLogs(_ context.Context, logs []MatchLog) error {
	f.logs = append(f.logs, logs...)
	return nil
}

func TestWriterForwardsLogsToStore(t *testing.T) {
	store := &fakeStore{}
	writer := NewWriter(store)
	in := make(chan []MatchLog, 1)
	in <- []MatchLog{{Ticker: "BTC-USD"}}
	close(in)

	writer.Run(context.Background(), in)

	if len(store.logs) != 1 {
		t.Fatalf("stored logs want 1, got %d", len(store.logs))
	}
	if store.logs[0].Ticker != "BTC-USD" {
		t.Fatalf("unexpected stored log: %#v", store.logs[0])
	}
}
