package engine

import (
	"log/slog"
	"math"

	"github.com/nogie-dev/clob-trading/internal/models"
)

// Match consumes an incoming order against the provided orderbook.
// It should execute fills while bestBid >= bestAsk and return any residual portion
// that needs to rest on the book (or nil if fully filled).
func Match(book *OrderBook, incoming *models.BookOrder) *models.BookOrder {
	if book == nil || incoming == nil {
		return incoming
	}

	// 오더북에 등록된 주문과 최신 주문 비교
	switch incoming.Position {
	case models.Bid:
		for {
			bestAsk := book.askLevels.Peek()
			if bestAsk == nil || incoming.Amount <= 0 || incoming.Price < bestAsk.Price {
				break
			}
			elem := bestAsk.Queue.Front()
			if elem == nil {
				break
			}
			target, ok := elem.Value.(*models.BookOrder)
			if !ok || target == nil {
				slog.Error("unexpected queue element type", "value", elem.Value)
				break
			}

			tradeAmt := math.Min(incoming.Amount, target.Amount)
			logTradeExecuted(incoming.Ticker, incoming.OrderID, target.OrderID, bestAsk.Price, tradeAmt)
			incoming.Amount -= tradeAmt
			target.Amount -= tradeAmt
			bestAsk.TotalAmount -= tradeAmt

			if target.Amount <= 0 {
				bestAsk.Queue.Remove(elem)
			}
			if bestAsk.Queue.Len() == 0 {
				book.dropPriceLevel(&book.askLevels, book.Asks, bestAsk)
			}
		}

	case models.Ask:
		for {
			bestBid := book.bidLevels.Peek()
			if bestBid == nil || incoming.Amount <= 0 || incoming.Price > bestBid.Price {
				break
			}
			elem := bestBid.Queue.Front()
			if elem == nil {
				break
			}
			target, ok := elem.Value.(*models.BookOrder)
			if !ok || target == nil {
				slog.Error("unexpected queue element type", "value", elem.Value)
				break
			}

			tradeAmt := math.Min(incoming.Amount, target.Amount)
			logTradeExecuted(incoming.Ticker, incoming.OrderID, target.OrderID, bestBid.Price, tradeAmt)
			incoming.Amount -= tradeAmt
			target.Amount -= tradeAmt
			bestBid.TotalAmount -= tradeAmt

			if target.Amount <= 0 {
				bestBid.Queue.Remove(elem)
			}
			if bestBid.Queue.Len() == 0 {
				book.dropPriceLevel(&book.bidLevels, book.Bids, bestBid)
			}
		}
	}

	if incoming.Amount <= 0 {
		return nil
	}
	return incoming
}
