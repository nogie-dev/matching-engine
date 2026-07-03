package engine

import (
	"log/slog"

	"github.com/nogie-dev/clob-trading/internal/matchlog"
)

const DefaultWorkerInputBufferSize = 128

type BookWorkerOptions struct {
	InputBufferSize int
	MatchLogOut     chan<- []matchlog.MatchLog
}

type BookWorker struct {
	ticker      string
	OrderBook   *OrderBook
	in          chan Event
	matchLogOut chan<- []matchlog.MatchLog
}

// NewBookWorker owns one orderbook per ticker and consumes events over its input channel.
// If an orderbook is provided, the worker will reuse it; otherwise a new one is created.
func NewBookWorker(ticker string, ob *OrderBook) *BookWorker {
	return NewBookWorkerWithOptions(ticker, ob, BookWorkerOptions{})
}

func NewBookWorkerWithOptions(ticker string, ob *OrderBook, opts BookWorkerOptions) *BookWorker {
	if ob == nil {
		ob = NewOrderBook(ticker)
	}
	bufferSize := opts.InputBufferSize
	if bufferSize <= 0 {
		bufferSize = DefaultWorkerInputBufferSize
	}
	return &BookWorker{
		ticker:      ticker,
		OrderBook:   ob,
		in:          make(chan Event, bufferSize),
		matchLogOut: opts.MatchLogOut,
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
			result := Match(w.OrderBook, &order)
			w.emitMatchLogs(result.Logs)
			if result.Residual != nil {
				w.OrderBook.AddOrder(result.Residual)
				reason := "no_match"
				if result.Residual.Amount < originalAmount {
					reason = "partial_fill"
				}
				logOrderResting(result.Residual, reason)
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
				result := Match(w.OrderBook, updated)
				w.emitMatchLogs(result.Logs)
				if result.Residual != nil {
					w.OrderBook.AddOrder(result.Residual)
					reason := "no_match"
					if result.Residual.Amount < originalAmount {
						reason = "partial_fill"
					}
					logOrderResting(result.Residual, reason)
				}
			}
		default:
			slog.Warn("unsupported event type", "type", ev.Type)
		}
	}
}

func (w *BookWorker) emitMatchLogs(logs []matchlog.MatchLog) {
	if len(logs) == 0 || w.matchLogOut == nil {
		return
	}
	w.matchLogOut <- logs
}
