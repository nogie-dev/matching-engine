package engine

import "log/slog"

type BookWorker struct {
	ticker    string
	OrderBook *OrderBook
	in        chan Event
	// out       chan TradeEvent
}

// NewBookWorker owns one orderbook per ticker and consumes events over its input channel.
// If an orderbook is provided, the worker will reuse it; otherwise a new one is created.
func NewBookWorker(ticker string, ob *OrderBook) *BookWorker {
	if ob == nil {
		ob = NewOrderBook(ticker)
	}
	return &BookWorker{
		ticker:    ticker,
		OrderBook: ob,
		in:        make(chan Event, 128),
	}
}

// Run processes events until the channel is closed.
func (w *BookWorker) Run() {
	for ev := range w.in {
		// Basic ticker guard to avoid misroutes.
		if ev.Ticker != "" && ev.Ticker != w.ticker {
			slog.Warn("mismatched ticker", "got", ev.Ticker, "worker", w.ticker)
			continue
		}

		switch ev.Type {
		case NewOrder:
			if ev.NewOrder == nil {
				slog.Warn("nil NewOrder payload", "ticker", w.ticker)
				continue
			}
			if ev.NewOrder.Ticker != w.ticker {
				slog.Warn("mismatched NewOrder payload ticker",
					"eventTicker", ev.Ticker,
					"payloadTicker", ev.NewOrder.Ticker,
					"worker", w.ticker,
				)
				continue
			}
			order := CreateOrder(*ev.NewOrder)
			originalAmount := order.Amount
			logOrderReceived(&order)
			residual := Match(w.OrderBook, &order)
			if residual != nil {
				w.OrderBook.AddOrder(residual)
				reason := "no_match"
				if residual.Amount < originalAmount {
					reason = "partial_fill"
				}
				logOrderResting(residual, reason)
			}
		case CancelOrder:
			if ev.CancelReq == nil {
				slog.Warn("nil CancelRequest payload", "ticker", w.ticker)
				continue
			}
			if ev.CancelReq.Ticker != w.ticker {
				slog.Warn("mismatched CancelOrder payload ticker",
					"eventTicker", ev.Ticker,
					"payloadTicker", ev.CancelReq.Ticker,
					"worker", w.ticker,
					"orderID", ev.CancelReq.OrderID,
				)
				continue
			}
			w.OrderBook.RemoveOrder(ev.CancelReq.OrderID)
		case EditOrder:
			if ev.EditReq == nil {
				slog.Warn("nil EditOrderRequest payload", "ticker", w.ticker)
				continue
			}
			if ev.EditReq.Ticker != w.ticker {
				slog.Warn("mismatched EditOrder payload ticker",
					"eventTicker", ev.Ticker,
					"payloadTicker", ev.EditReq.Ticker,
					"worker", w.ticker,
					"orderID", ev.EditReq.OrderID,
				)
				continue
			}
			updated := w.OrderBook.EditOrder(*ev.EditReq)
			if updated != nil {
				originalAmount := updated.Amount
				residual := Match(w.OrderBook, updated)
				if residual != nil {
					w.OrderBook.AddOrder(residual)
					reason := "no_match"
					if residual.Amount < originalAmount {
						reason = "partial_fill"
					}
					logOrderResting(residual, reason)
				}
			}
		default:
			slog.Warn("unsupported event type", "type", ev.Type)
		}
	}
}
