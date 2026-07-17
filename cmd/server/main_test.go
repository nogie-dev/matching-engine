package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nogie-dev/clob-trading/internal/config"
	"github.com/nogie-dev/clob-trading/internal/engine"
	"github.com/nogie-dev/clob-trading/internal/matchlog"
	"github.com/nogie-dev/clob-trading/internal/models"
)

type recordingStore struct {
	logs []matchlog.MatchLog
	err  error
}

func (s *recordingStore) SaveMatchLog(ctx context.Context, log matchlog.MatchLog) error {
	return s.SaveMatchLogs(ctx, []matchlog.MatchLog{log})
}

func (s *recordingStore) SaveMatchLogs(_ context.Context, logs []matchlog.MatchLog) error {
	s.logs = append(s.logs, logs...)
	return s.err
}

func TestRequiredDatabaseURL(t *testing.T) {
	if _, err := requiredDatabaseURL(func(string) string { return " " }); err == nil {
		t.Fatal("requiredDatabaseURL should reject a missing URL")
	}

	url, err := requiredDatabaseURL(func(name string) string {
		if name != databaseURLEnv {
			t.Fatalf("environment key want %q, got %q", databaseURLEnv, name)
		}
		return " postgres://localhost/matching "
	})
	if err != nil {
		t.Fatalf("requiredDatabaseURL returned error: %v", err)
	}
	if url != "postgres://localhost/matching" {
		t.Fatalf("trimmed URL want postgres://localhost/matching, got %q", url)
	}
}

func TestEngineRuntimePersistsMatchBeforeShutdown(t *testing.T) {
	store := &recordingStore{}
	runtime, err := startEngine(config.Default(), store)
	if err != nil {
		t.Fatalf("startEngine returned error: %v", err)
	}

	if err := runtime.router.OrderRouter(engine.Event{
		Type:   engine.NewOrder,
		Ticker: "BTC-USD",
		NewOrder: &models.CreateOrderRequest{
			Ticker:    "BTC-USD",
			UserID:    "maker",
			OrderType: models.Limit,
			Position:  models.Ask,
			Price:     100,
			Amount:    1,
			Nonce:     1,
		},
	}); err != nil {
		t.Fatalf("route maker order: %v", err)
	}
	if err := runtime.router.OrderRouter(engine.Event{
		Type:   engine.NewOrder,
		Ticker: "BTC-USD",
		NewOrder: &models.CreateOrderRequest{
			Ticker:    "BTC-USD",
			UserID:    "taker",
			OrderType: models.Limit,
			Position:  models.Bid,
			Price:     100,
			Amount:    1,
			Nonce:     1,
		},
	}); err != nil {
		t.Fatalf("route taker order: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.shutdown(ctx); err != nil {
		t.Fatalf("runtime shutdown: %v", err)
	}
	if len(store.logs) != 1 {
		t.Fatalf("persisted logs want 1, got %d", len(store.logs))
	}
	if store.logs[0].ExecutionID == "" {
		t.Fatal("persisted log must have an execution ID")
	}
}

func TestEngineRuntimeHaltsOnStoreFailure(t *testing.T) {
	storeErr := errors.New("database unavailable")
	runtime, err := startEngine(config.Default(), &recordingStore{err: storeErr})
	if err != nil {
		t.Fatalf("startEngine returned error: %v", err)
	}

	orders := []models.CreateOrderRequest{
		{Ticker: "BTC-USD", UserID: "maker", OrderType: models.Limit, Position: models.Ask, Price: 100, Amount: 1, Nonce: 1},
		{Ticker: "BTC-USD", UserID: "taker", OrderType: models.Limit, Position: models.Bid, Price: 100, Amount: 1, Nonce: 1},
	}
	for i := range orders {
		if err := runtime.router.OrderRouter(engine.Event{Type: engine.NewOrder, Ticker: "BTC-USD", NewOrder: &orders[i]}); err != nil {
			t.Fatalf("route order %d: %v", i, err)
		}
	}

	deadline := time.After(time.Second)
	for !errors.Is(runtime.router.Ready(), engine.ErrEngineHalted) {
		select {
		case <-deadline:
			t.Fatal("runtime did not halt after store failure")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if err := runtime.router.OrderRouter(engine.Event{Type: engine.CancelOrder, Ticker: "BTC-USD"}); !errors.Is(err, engine.ErrEngineHalted) {
		t.Fatalf("halted runtime command want ErrEngineHalted, got %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.shutdown(ctx); err != nil {
		t.Fatalf("runtime shutdown: %v", err)
	}
}
