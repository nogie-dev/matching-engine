package engine

import "log"

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
			log.Printf("mismatched ticker: got %s, worker %s", ev.Ticker, w.ticker)
			continue
		}

		switch ev.Type {
		case NewOrder:
			if ev.NewOrder == nil {
				log.Printf("nil NewOrder payload")
				continue
			}
			order := CreateOrder(*ev.NewOrder)
			w.OrderBook.AddOrder(&order)
		case CancelOrder:
			if ev.CancelReq == nil {
				log.Printf("nil CancelRequest payload")
				continue
			}
			w.OrderBook.RemoveOrder(ev.CancelReq.OrderID)
		case EditOrder:
			if ev.EditReq == nil {
				log.Printf("nil EditOrderRequest payload")
				continue
			}
			w.OrderBook.EditOrder(*ev.EditReq)
		default:
			log.Printf("unsupported event type: %v", ev.Type)
		}
	}
}
