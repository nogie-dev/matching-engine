package engine

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrEmptyTicker   = errors.New("empty ticker")
	ErrUnknownTicker = errors.New("unknown ticker")
)

// Router holds book-specific workers to dispatch incoming events/orders.
type Router struct {
	mu      sync.RWMutex
	workers map[string]*BookWorker
}

// NewRouter initializes an empty router with no workers.
func NewRouter() *Router {
	return &Router{
		workers: make(map[string]*BookWorker),
	}
}

// Register adds a pre-created worker for a ticker.
// Use at startup after restoring orderbooks from storage.
func (r *Router) Register(ticker string, w *BookWorker) error {
	if ticker == "" {
		return ErrEmptyTicker
	}
	if w == nil {
		return fmt.Errorf("nil worker for ticker %s", ticker)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers[ticker] = w
	return nil
}

func (r *Router) OrderRouter(ev Event) error {
	w, err := r.worker(ev.Ticker)
	if err != nil {
		return err
	}

	w.in <- ev
	return nil
}

// OrderBookSnapshot reads a snapshot on the owning worker's event queue.
func (r *Router) OrderBookSnapshot(ticker string, depth int) (OrderBookSnapshot, error) {
	w, err := r.worker(ticker)
	if err != nil {
		return OrderBookSnapshot{}, err
	}

	result := make(chan OrderBookSnapshot, 1)
	w.in <- Event{
		Type:           snapshotOrderBook,
		Ticker:         ticker,
		snapshotDepth:  depth,
		snapshotResult: result,
	}
	return <-result, nil
}

func (r *Router) worker(ticker string) (*BookWorker, error) {
	if ticker == "" {
		return nil, ErrEmptyTicker
	}

	r.mu.RLock()
	w := r.workers[ticker]
	r.mu.RUnlock()
	if w == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTicker, ticker)
	}
	return w, nil
}
