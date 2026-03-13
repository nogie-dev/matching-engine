package engine

import (
	"math"
	"testing"

	"github.com/nogie-dev/clob-trading/internal/models"
)

const epsilon = 1e-9

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

// --- 가격 불일치: 체결 없음 ---

// BID 가격이 best ask보다 낮으면 체결되지 않고 잔여분이 그대로 반환된다.
func TestMatchNoMatch_BidBelowAsk(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	ob.AddOrder(newOrder("ask-1", models.Ask, 101, 1.0))

	taker := newOrder("bid-1", models.Bid, 100, 1.0)
	residual := Match(ob, taker)

	if residual == nil {
		t.Fatal("expected no match, got full fill")
	}
	if residual.Amount != 1.0 {
		t.Fatalf("residual amount want 1.0, got %v", residual.Amount)
	}
	if _, ok := ob.Asks[101]; !ok {
		t.Fatal("ask level should remain on book")
	}
}

// ASK 가격이 best bid보다 높으면 체결되지 않는다.
func TestMatchNoMatch_AskAboveBid(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	ob.AddOrder(newOrder("bid-1", models.Bid, 100, 1.0))

	taker := newOrder("ask-1", models.Ask, 101, 1.0)
	residual := Match(ob, taker)

	if residual == nil {
		t.Fatal("expected no match, got full fill")
	}
	if residual.Amount != 1.0 {
		t.Fatalf("residual amount want 1.0, got %v", residual.Amount)
	}
	if _, ok := ob.Bids[100]; !ok {
		t.Fatal("bid level should remain on book")
	}
}

// --- 완전 체결 ---

// BID taker가 동일 수량의 ASK maker와 완전 체결된다.
func TestMatchFullFill_BidTaker(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	maker := newOrder("ask-1", models.Ask, 100, 1.0)
	ob.AddOrder(maker)

	taker := newOrder("bid-1", models.Bid, 100, 1.0)
	residual := Match(ob, taker)

	if residual != nil {
		t.Fatalf("expected full fill, got residual amount %v", residual.Amount)
	}
	if maker.Amount != 0 {
		t.Fatalf("maker should be fully filled, got %v", maker.Amount)
	}
	if len(ob.Asks) != 0 {
		t.Fatal("ask book should be empty after full fill")
	}
}

// ASK taker가 동일 수량의 BID maker와 완전 체결된다.
func TestMatchFullFill_AskTaker(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	maker := newOrder("bid-1", models.Bid, 100, 1.0)
	ob.AddOrder(maker)

	taker := newOrder("ask-1", models.Ask, 100, 1.0)
	residual := Match(ob, taker)

	if residual != nil {
		t.Fatalf("expected full fill, got residual amount %v", residual.Amount)
	}
	if maker.Amount != 0 {
		t.Fatalf("maker should be fully filled, got %v", maker.Amount)
	}
	if len(ob.Bids) != 0 {
		t.Fatal("bid book should be empty after full fill")
	}
}

// --- 부분 체결: taker 수량 > maker 수량 ---

// BID taker 수량이 ASK maker보다 많으면 maker가 소진되고 taker 잔여분이 북에 올라간다.
func TestMatchPartialFill_TakerLarger(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	maker := newOrder("ask-1", models.Ask, 100, 0.3)
	ob.AddOrder(maker)

	taker := newOrder("bid-1", models.Bid, 100, 1.0)
	residual := Match(ob, taker)

	if residual == nil {
		t.Fatal("expected partial fill residual, got nil")
	}
	wantResidual := 0.7
	if residual.Amount != wantResidual {
		t.Fatalf("residual amount want %v, got %v", wantResidual, residual.Amount)
	}
	if maker.Amount != 0 {
		t.Fatalf("maker should be fully consumed, got %v", maker.Amount)
	}
	if len(ob.Asks) != 0 {
		t.Fatal("ask book should be empty after maker consumed")
	}
}

// --- 부분 체결: maker 수량 > taker 수량 ---

// BID taker 수량이 ASK maker보다 적으면 taker가 완전 체결되고 maker 잔여분이 북에 남는다.
func TestMatchPartialFill_MakerLarger(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	maker := newOrder("ask-1", models.Ask, 100, 1.0)
	ob.AddOrder(maker)

	taker := newOrder("bid-1", models.Bid, 100, 0.4)
	residual := Match(ob, taker)

	if residual != nil {
		t.Fatalf("taker should be fully filled, got residual %v", residual.Amount)
	}
	wantMakerRemain := 0.6
	if maker.Amount != wantMakerRemain {
		t.Fatalf("maker remaining want %v, got %v", wantMakerRemain, maker.Amount)
	}
	lvl, ok := ob.Asks[100]
	if !ok {
		t.Fatal("ask level should remain on book")
	}
	if lvl.TotalAmount != wantMakerRemain {
		t.Fatalf("ask level total want %v, got %v", wantMakerRemain, lvl.TotalAmount)
	}
}

// --- 멀티 레벨 소진 ---

// BID taker 가격이 여러 ASK 레벨을 커버하면 낮은 가격부터 순서대로 소진된다.
func TestMatchMultiLevel_BidSweepsAsks(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	ob.AddOrder(newOrder("ask-100", models.Ask, 100, 0.3))
	ob.AddOrder(newOrder("ask-101", models.Ask, 101, 0.3))
	ob.AddOrder(newOrder("ask-102", models.Ask, 102, 0.3))

	// 101까지만 커버하는 BID
	taker := newOrder("bid-1", models.Bid, 101, 0.5)
	residual := Match(ob, taker)

	if residual != nil {
		t.Fatalf("taker should be fully filled, got residual %v", residual.Amount)
	}
	if _, ok := ob.Asks[100]; ok {
		t.Fatal("ask@100 should be fully consumed")
	}
	lvl101, ok := ob.Asks[101]
	if !ok {
		t.Fatal("ask@101 should remain partially")
	}
	wantRemain := 0.1
	if !approxEqual(lvl101.TotalAmount, wantRemain) {
		t.Fatalf("remaining want %v, got %v", wantRemain, lvl101.TotalAmount)
	}
	if _, ok := ob.Asks[102]; !ok {
		t.Fatal("ask@102 should be untouched")
	}
}

// ASK taker 가격이 여러 BID 레벨을 커버하면 높은 가격부터 순서대로 소진된다.
func TestMatchMultiLevel_AskSweepsBids(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	ob.AddOrder(newOrder("bid-102", models.Bid, 102, 0.3))
	ob.AddOrder(newOrder("bid-101", models.Bid, 101, 0.3))
	ob.AddOrder(newOrder("bid-100", models.Bid, 100, 0.3))

	// 101까지만 커버하는 ASK
	taker := newOrder("ask-1", models.Ask, 101, 0.5)
	residual := Match(ob, taker)

	if residual != nil {
		t.Fatalf("taker should be fully filled, got residual %v", residual.Amount)
	}
	if _, ok := ob.Bids[102]; ok {
		t.Fatal("bid@102 should be fully consumed")
	}
	lvl101, ok := ob.Bids[101]
	if !ok {
		t.Fatal("bid@101 should remain partially")
	}
	wantRemain := 0.1
	if !approxEqual(lvl101.TotalAmount, wantRemain) {
		t.Fatalf("remaining want %v, got %v", wantRemain, lvl101.TotalAmount)
	}
	if _, ok := ob.Bids[100]; !ok {
		t.Fatal("bid@100 should be untouched")
	}
}

// --- 가격 우선 ---

// 동일 BID taker에 대해 여러 ASK 레벨이 있을 때 가장 낮은 가격(best ask)부터 체결된다.
func TestMatchPricePriority(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	// 101을 먼저 추가했더라도 100이 best ask
	ob.AddOrder(newOrder("ask-101", models.Ask, 101, 1.0))
	ob.AddOrder(newOrder("ask-100", models.Ask, 100, 1.0))

	taker := newOrder("bid-1", models.Bid, 102, 1.0)
	residual := Match(ob, taker)

	if residual != nil {
		t.Fatalf("taker should be fully filled, got residual %v", residual.Amount)
	}
	// 100이 먼저 소진돼야 함
	if _, ok := ob.Asks[100]; ok {
		t.Fatal("ask@100 (best price) should be consumed first")
	}
	if _, ok := ob.Asks[101]; !ok {
		t.Fatal("ask@101 should remain untouched")
	}
}

// --- 시간 우선 (FIFO) ---

// 같은 가격의 ASK 주문이 여러 개일 때 먼저 들어온 주문이 먼저 체결된다.
func TestMatchTimePriority_FIFO(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	first := newOrder("ask-first", models.Ask, 100, 0.5)
	second := newOrder("ask-second", models.Ask, 100, 0.5)
	ob.AddOrder(first)
	ob.AddOrder(second)

	taker := newOrder("bid-1", models.Bid, 100, 0.5)
	residual := Match(ob, taker)

	if residual != nil {
		t.Fatalf("taker should be fully filled, got residual %v", residual.Amount)
	}
	// first가 소진되고 second는 그대로
	if first.Amount != 0 {
		t.Fatalf("first order should be consumed, got amount %v", first.Amount)
	}
	if second.Amount != 0.5 {
		t.Fatalf("second order should be untouched, got amount %v", second.Amount)
	}
}
