package engine

import (
	"testing"
	"time"

	"github.com/nogie-dev/clob-trading/internal/matchlog"
	"github.com/nogie-dev/clob-trading/internal/models"
)

func routeAndDrain(t *testing.T, router *Router, worker *BookWorker, ev Event) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		worker.Run()
		close(done)
	}()

	if err := router.OrderRouter(ev); err != nil {
		t.Fatalf("OrderRouter returned error: %v", err)
	}
	close(worker.in)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not drain routed event before timeout")
	}
}

func TestBookWorkerRejectsNewOrderPayloadTickerMismatch(t *testing.T) {
	worker := NewBookWorker("BTC-USD", nil)
	router := NewRouter()
	if err := router.Register("BTC-USD", worker); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	routeAndDrain(t, router, worker, Event{
		Type:   NewOrder,
		Ticker: "BTC-USD",
		NewOrder: &models.CreateOrderRequest{
			Ticker:    "ETH-USD",
			UserID:    "alice",
			OrderType: models.Limit,
			Position:  models.Bid,
			Price:     100,
			Amount:    1,
			Nonce:     1,
		},
	})

	if len(worker.OrderBook.Bids) != 0 {
		t.Fatalf("mismatched NewOrder payload should not create bids, got %d levels", len(worker.OrderBook.Bids))
	}
	if len(worker.OrderBook.Asks) != 0 {
		t.Fatalf("mismatched NewOrder payload should not create asks, got %d levels", len(worker.OrderBook.Asks))
	}
	if len(worker.OrderBook.Index) != 0 {
		t.Fatalf("mismatched NewOrder payload should not index orders, got %d entries", len(worker.OrderBook.Index))
	}
}

func TestBookWorkerEmitsMatchLogs(t *testing.T) {
	logOut := make(chan []matchlog.MatchLog, 1)
	worker := NewBookWorkerWithOptions("BTC-USD", nil, BookWorkerOptions{
		MatchLogOut: logOut,
	})
	maker := newOrder("ask-1", models.Ask, 100, 0.5)
	maker.UserID = "maker-user"
	worker.OrderBook.AddOrder(maker)

	router := NewRouter()
	if err := router.Register("BTC-USD", worker); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	routeAndDrain(t, router, worker, Event{
		Type:   NewOrder,
		Ticker: "BTC-USD",
		NewOrder: &models.CreateOrderRequest{
			Ticker:    "BTC-USD",
			UserID:    "taker-user",
			OrderType: models.Limit,
			Position:  models.Bid,
			Price:     101,
			Amount:    0.25,
			Nonce:     1,
		},
	})

	var logs []matchlog.MatchLog
	select {
	case logs = <-logOut:
	default:
		t.Fatal("expected emitted match logs")
	}

	if len(logs) != 1 {
		t.Fatalf("emitted match logs want 1, got %d", len(logs))
	}
	if logs[0].MakerOrderID != "ask-1" || logs[0].TakerUserID != "taker-user" {
		t.Fatalf("unexpected emitted match log: %#v", logs[0])
	}
}

func TestNewBookWorkerWithOptionsUsesInputBufferSize(t *testing.T) {
	worker := NewBookWorkerWithOptions("BTC-USD", nil, BookWorkerOptions{
		InputBufferSize: 3,
	})

	if cap(worker.in) != 3 {
		t.Fatalf("worker input channel cap want 3, got %d", cap(worker.in))
	}
}

func TestBookWorkerRejectsCancelOrderPayloadTickerMismatch(t *testing.T) {
	worker := NewBookWorker("BTC-USD", nil)
	existing := newOrder("bid-1", models.Bid, 100, 1)
	worker.OrderBook.AddOrder(existing)

	router := NewRouter()
	if err := router.Register("BTC-USD", worker); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	routeAndDrain(t, router, worker, Event{
		Type:   CancelOrder,
		Ticker: "BTC-USD",
		CancelReq: &models.CancelOrderRequest{
			Ticker:  "ETH-USD",
			OrderID: existing.OrderID,
		},
	})

	if _, ok := worker.OrderBook.Index[existing.OrderID]; !ok {
		t.Fatalf("mismatched CancelOrder payload should not remove order %q from index", existing.OrderID)
	}
	if _, ok := worker.OrderBook.Bids[existing.Price]; !ok {
		t.Fatalf("mismatched CancelOrder payload should not remove bid level %.2f", existing.Price)
	}
}

func TestBookWorkerRejectsEditOrderPayloadTickerMismatch(t *testing.T) {
	worker := NewBookWorker("BTC-USD", nil)
	existing := newOrder("bid-1", models.Bid, 100, 1)
	worker.OrderBook.AddOrder(existing)

	router := NewRouter()
	if err := router.Register("BTC-USD", worker); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	newAmount := 2.0
	routeAndDrain(t, router, worker, Event{
		Type:   EditOrder,
		Ticker: "BTC-USD",
		EditReq: &models.EditOrderRequest{
			Ticker:  "ETH-USD",
			OrderID: existing.OrderID,
			Price:   101,
			Amount:  &newAmount,
		},
	})

	if existing.Price != 100 {
		t.Fatalf("mismatched EditOrder payload should not change price, got %.2f", existing.Price)
	}
	if existing.Amount != 1 {
		t.Fatalf("mismatched EditOrder payload should not change amount, got %.2f", existing.Amount)
	}
	if _, ok := worker.OrderBook.Bids[100]; !ok {
		t.Fatal("mismatched EditOrder payload should keep original bid level")
	}
	if _, ok := worker.OrderBook.Bids[101]; ok {
		t.Fatal("mismatched EditOrder payload should not create edited bid level")
	}
}

func TestRouterOrderBookSnapshotRunsAfterQueuedCommand(t *testing.T) {
	worker := NewBookWorker("BTC-USD", nil)
	router := NewRouter()
	if err := router.Register("BTC-USD", worker); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		worker.Run()
		close(done)
	}()

	if err := router.OrderRouter(Event{
		Type:   NewOrder,
		Ticker: "BTC-USD",
		NewOrder: &models.CreateOrderRequest{
			Ticker:    "BTC-USD",
			UserID:    "alice",
			OrderType: models.Limit,
			Position:  models.Bid,
			Price:     100,
			Amount:    2,
			Nonce:     1,
		},
	}); err != nil {
		t.Fatalf("OrderRouter returned error: %v", err)
	}

	snapshot, err := router.OrderBookSnapshot("BTC-USD", 1)
	if err != nil {
		t.Fatalf("OrderBookSnapshot returned error: %v", err)
	}
	assertLevel(t, snapshot.Bids, 0, 100, 2, 2)

	close(worker.in)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop before timeout")
	}
}

func TestRouterOrderBookSnapshotRejectsUnknownTicker(t *testing.T) {
	router := NewRouter()

	if _, err := router.OrderBookSnapshot("ETH-USD", 1); err == nil {
		t.Fatal("OrderBookSnapshot should reject an unknown ticker")
	}
}
