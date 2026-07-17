package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nogie-dev/clob-trading/internal/config"
	"github.com/nogie-dev/clob-trading/internal/engine"
	"github.com/nogie-dev/clob-trading/internal/journal"
	"github.com/nogie-dev/clob-trading/internal/matchlog"
	"github.com/nogie-dev/clob-trading/internal/models"
)

type recordingStore struct {
	logs []matchlog.MatchLog
	err  error
}

type idempotentStore struct {
	logs map[string]matchlog.MatchLog
	err  error
}

func newIdempotentStore() *idempotentStore {
	return &idempotentStore{logs: make(map[string]matchlog.MatchLog)}
}

func (s *idempotentStore) SaveMatchLog(ctx context.Context, log matchlog.MatchLog) error {
	return s.SaveMatchLogs(ctx, []matchlog.MatchLog{log})
}

func (s *idempotentStore) SaveMatchLogs(_ context.Context, logs []matchlog.MatchLog) error {
	if s.err != nil {
		return s.err
	}
	for _, log := range logs {
		if existing, ok := s.logs[log.ExecutionID]; ok && !reflect.DeepEqual(existing, log) {
			return errors.New("execution id conflict")
		}
	}
	for _, log := range logs {
		s.logs[log.ExecutionID] = log
	}
	return nil
}

type memoryJournal struct {
	commands []journal.Command
	err      error
}

func (m *memoryJournal) Append(_ context.Context, command journal.Command) (journal.AppendResult, error) {
	if m.err != nil {
		return journal.AppendResult{}, m.err
	}
	for _, existing := range m.commands {
		if existing.CommandID != command.CommandID {
			continue
		}
		if !journal.SamePayload(existing, command) {
			return journal.AppendResult{}, journal.ErrCommandConflict
		}
		return journal.AppendResult{Command: existing, Inserted: false}, nil
	}
	var sequence int64 = 1
	for _, existing := range m.commands {
		if existing.Ticker == command.Ticker && existing.Sequence >= sequence {
			sequence = existing.Sequence + 1
		}
	}
	command.Sequence = sequence
	command.RecordedAt = time.Date(2026, 7, 17, 12, 0, 0, int(sequence), time.UTC)
	m.commands = append(m.commands, command)
	return journal.AppendResult{Command: command, Inserted: true}, nil
}

func (m *memoryJournal) List(context.Context) ([]journal.Command, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]journal.Command(nil), m.commands...), nil
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

func TestStartEngineFailsBeforeServingWhenJournalLoadFails(t *testing.T) {
	journalErr := errors.New("journal unavailable")
	_, err := startEngine(
		context.Background(),
		config.Default(),
		newIdempotentStore(),
		&memoryJournal{err: journalErr},
	)
	if !errors.Is(err, journalErr) {
		t.Fatalf("startEngine want journal load error, got %v", err)
	}
}

func TestEngineRuntimePersistsMatchBeforeShutdown(t *testing.T) {
	store := &recordingStore{}
	runtime, err := startEngine(context.Background(), config.Default(), store, &memoryJournal{})
	if err != nil {
		t.Fatalf("startEngine returned error: %v", err)
	}

	if err := runtime.router.OrderRouter(engine.Event{
		Type:   engine.NewOrder,
		Ticker: "BTC-USD",
		NewOrder: &models.CreateOrderRequest{
			CommandID: "maker-command",
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
			CommandID: "taker-command",
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
	runtime, err := startEngine(context.Background(), config.Default(), &recordingStore{err: storeErr}, &memoryJournal{})
	if err != nil {
		t.Fatalf("startEngine returned error: %v", err)
	}

	orders := []models.CreateOrderRequest{
		{CommandID: "maker-command", Ticker: "BTC-USD", UserID: "maker", OrderType: models.Limit, Position: models.Ask, Price: 100, Amount: 1, Nonce: 1},
		{CommandID: "taker-command", Ticker: "BTC-USD", UserID: "taker", OrderType: models.Limit, Position: models.Bid, Price: 100, Amount: 1, Nonce: 1},
	}
	if err := runtime.router.OrderRouter(engine.Event{Type: engine.NewOrder, Ticker: "BTC-USD", NewOrder: &orders[0]}); err != nil {
		t.Fatalf("route maker order: %v", err)
	}
	if err := runtime.router.OrderRouter(engine.Event{Type: engine.NewOrder, Ticker: "BTC-USD", NewOrder: &orders[1]}); !errors.Is(err, engine.ErrEngineHalted) {
		t.Fatalf("failed taker order want ErrEngineHalted, got %v", err)
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

func TestStartEngineReplaysJournalAndReconcilesMatchLogs(t *testing.T) {
	journalStore := &memoryJournal{}
	commands := []journal.Command{
		{
			CommandID: "maker-command", Ticker: "BTC-USD", Type: journal.CreateCommand,
			Create: &models.CreateOrderRequest{CommandID: "maker-command", Ticker: "BTC-USD", UserID: "maker", OrderType: models.Limit, Position: models.Ask, Price: 100, Amount: 1, Nonce: 1},
		},
		{
			CommandID: "taker-command", Ticker: "BTC-USD", Type: journal.CreateCommand,
			Create: &models.CreateOrderRequest{CommandID: "taker-command", Ticker: "BTC-USD", UserID: "taker", OrderType: models.Limit, Position: models.Bid, Price: 100, Amount: 1, Nonce: 1},
		},
	}
	for _, command := range commands {
		if _, err := journalStore.Append(context.Background(), command); err != nil {
			t.Fatalf("append recovery command: %v", err)
		}
	}
	matchStore := newIdempotentStore()

	for attempt := 1; attempt <= 2; attempt++ {
		runtime, err := startEngine(context.Background(), config.Default(), matchStore, journalStore)
		if err != nil {
			t.Fatalf("startEngine replay attempt %d: %v", attempt, err)
		}
		if err := runtime.router.Ready(); err != nil {
			t.Fatalf("runtime should be ready after replay attempt %d: %v", attempt, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := runtime.shutdown(ctx); err != nil {
			cancel()
			t.Fatalf("runtime shutdown attempt %d: %v", attempt, err)
		}
		cancel()
	}
	if len(matchStore.logs) != 1 {
		t.Fatalf("replaying the same journal twice must keep one match log, got %d", len(matchStore.logs))
	}
}

func TestRestartRecoversJournaledCommandAfterMatchLogFailure(t *testing.T) {
	journalStore := &memoryJournal{}
	matchStore := newIdempotentStore()
	runtime, err := startEngine(context.Background(), config.Default(), matchStore, journalStore)
	if err != nil {
		t.Fatalf("startEngine returned error: %v", err)
	}

	maker := models.CreateOrderRequest{CommandID: "maker-command", Ticker: "BTC-USD", UserID: "maker", OrderType: models.Limit, Position: models.Ask, Price: 100, Amount: 1, Nonce: 1}
	if err := runtime.router.OrderRouter(engine.Event{Type: engine.NewOrder, Ticker: maker.Ticker, NewOrder: &maker}); err != nil {
		t.Fatalf("route maker order: %v", err)
	}
	matchStore.err = errors.New("match log unavailable")
	taker := models.CreateOrderRequest{CommandID: "taker-command", Ticker: "BTC-USD", UserID: "taker", OrderType: models.Limit, Position: models.Bid, Price: 100, Amount: 1, Nonce: 1}
	if err := runtime.router.OrderRouter(engine.Event{Type: engine.NewOrder, Ticker: taker.Ticker, NewOrder: &taker}); !errors.Is(err, engine.ErrEngineHalted) {
		t.Fatalf("failed taker order want ErrEngineHalted, got %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := runtime.shutdown(ctx); err != nil {
		cancel()
		t.Fatalf("failed runtime shutdown: %v", err)
	}
	cancel()

	matchStore.err = nil
	recovered, err := startEngine(context.Background(), config.Default(), matchStore, journalStore)
	if err != nil {
		t.Fatalf("restart replay returned error: %v", err)
	}
	if len(matchStore.logs) != 1 {
		t.Fatalf("restart should recover one missing match log, got %d", len(matchStore.logs))
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := recovered.shutdown(ctx); err != nil {
		t.Fatalf("recovered runtime shutdown: %v", err)
	}
}

func TestEngineRuntimeDoesNotApplyDuplicateCommandTwice(t *testing.T) {
	runtime, err := startEngine(context.Background(), config.Default(), newIdempotentStore(), &memoryJournal{})
	if err != nil {
		t.Fatalf("startEngine returned error: %v", err)
	}
	request := models.CreateOrderRequest{
		CommandID: "create-command", Ticker: "BTC-USD", UserID: "alice",
		OrderType: models.Limit, Position: models.Bid, Price: 100, Amount: 1, Nonce: 1,
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := runtime.router.OrderRouter(engine.Event{Type: engine.NewOrder, Ticker: request.Ticker, NewOrder: &request}); err != nil {
			t.Fatalf("route duplicate attempt %d: %v", attempt, err)
		}
	}
	snapshot, err := runtime.router.OrderBookSnapshot("BTC-USD", 1)
	if err != nil {
		t.Fatalf("OrderBookSnapshot returned error: %v", err)
	}
	if len(snapshot.Bids) != 1 || snapshot.Bids[0].Amount != 1 {
		t.Fatalf("duplicate command changed the book twice: %#v", snapshot.Bids)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.shutdown(ctx); err != nil {
		t.Fatalf("runtime shutdown: %v", err)
	}
}
