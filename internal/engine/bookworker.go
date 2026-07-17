package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nogie-dev/clob-trading/internal/journal"
	"github.com/nogie-dev/clob-trading/internal/matchlog"
)

const DefaultWorkerInputBufferSize = 128

var ErrInvalidReplaySequence = errors.New("invalid replay sequence")

type BookWorkerOptions struct {
	InputBufferSize int
	PersistenceOut  chan<- matchlog.PersistenceRequest
	Journal         journal.Store
	State           *EngineState
}

type BookWorker struct {
	ticker         string
	OrderBook      *OrderBook
	in             chan Event
	persistenceOut chan<- matchlog.PersistenceRequest
	journalStore   journal.Store
	state          *EngineState
	done           chan struct{}
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
	state := opts.State
	if state == nil {
		state = NewEngineState()
	}
	return &BookWorker{
		ticker:         ticker,
		OrderBook:      ob,
		in:             make(chan Event, bufferSize),
		persistenceOut: opts.PersistenceOut,
		journalStore:   opts.Journal,
		state:          state,
		done:           make(chan struct{}),
	}
}

// Run processes events until the channel is closed.
func (w *BookWorker) Run() {
	defer close(w.done)
	for ev := range w.in {
		if isCommandEvent(ev.Type) && w.state.Err() != nil {
			ev.acknowledge(w.state.Err())
			continue
		}

		// Basic ticker guard to avoid misroutes.
		if ev.Ticker != "" && ev.Ticker != w.ticker {
			slog.Warn("mismatched ticker", "got", ev.Ticker, "worker", w.ticker)
			ev.acknowledge(fmt.Errorf("mismatched ticker %q for worker %q", ev.Ticker, w.ticker))
			continue
		}

		if ev.Type == snapshotOrderBook {
			if ev.snapshotResult != nil {
				ev.snapshotResult <- w.OrderBook.Snapshot(ev.snapshotDepth)
			}
			continue
		}

		command, ok := w.commandFromEvent(ev)
		if !ok {
			ev.acknowledge(nil)
			continue
		}
		if w.journalStore == nil {
			w.halt(journal.ErrStoreUnavailable)
			ev.acknowledge(w.state.Err())
			continue
		}
		appended, err := w.journalStore.Append(context.Background(), command)
		if err != nil {
			w.halt(err)
			ev.acknowledge(w.state.Err())
			continue
		}
		if !appended.Inserted {
			ev.acknowledge(nil)
			continue
		}
		if err := w.applyCommand(appended.Command); err != nil {
			w.halt(err)
			ev.acknowledge(w.state.Err())
			continue
		}
		ev.acknowledge(nil)
	}
}

func (w *BookWorker) Replay(commands []journal.Command) error {
	var lastSequence int64
	for i, command := range commands {
		if command.Ticker != w.ticker || command.Sequence <= lastSequence {
			err := fmt.Errorf("%w at row %d for ticker %q", ErrInvalidReplaySequence, i, command.Ticker)
			w.state.Halt(err)
			return err
		}
		if err := w.applyCommand(command); err != nil {
			w.state.Halt(err)
			return fmt.Errorf("replay command %q: %w", command.CommandID, err)
		}
		lastSequence = command.Sequence
	}
	return nil
}

func (w *BookWorker) commandFromEvent(ev Event) (journal.Command, bool) {
	switch ev.Type {
	case NewOrder:
		if ev.NewOrder == nil {
			slog.Warn("nil NewOrder payload", "ticker", w.ticker)
			return journal.Command{}, false
		}
		if ev.NewOrder.Ticker != w.ticker {
			slog.Warn("mismatched NewOrder payload ticker", "payloadTicker", ev.NewOrder.Ticker, "worker", w.ticker)
			return journal.Command{}, false
		}
		return journal.Command{CommandID: ev.NewOrder.CommandID, Ticker: w.ticker, Type: journal.CreateCommand, Create: ev.NewOrder}, true
	case EditOrder:
		if ev.EditReq == nil {
			slog.Warn("nil EditOrderRequest payload", "ticker", w.ticker)
			return journal.Command{}, false
		}
		if ev.EditReq.Ticker != w.ticker {
			slog.Warn("mismatched EditOrder payload ticker", "payloadTicker", ev.EditReq.Ticker, "worker", w.ticker, "orderID", ev.EditReq.OrderID)
			return journal.Command{}, false
		}
		return journal.Command{CommandID: ev.EditReq.CommandID, Ticker: w.ticker, Type: journal.AmendCommand, Amend: ev.EditReq}, true
	case CancelOrder:
		if ev.CancelReq == nil {
			slog.Warn("nil CancelRequest payload", "ticker", w.ticker)
			return journal.Command{}, false
		}
		if ev.CancelReq.Ticker != w.ticker {
			slog.Warn("mismatched CancelOrder payload ticker", "payloadTicker", ev.CancelReq.Ticker, "worker", w.ticker, "orderID", ev.CancelReq.OrderID)
			return journal.Command{}, false
		}
		return journal.Command{CommandID: ev.CancelReq.CommandID, Ticker: w.ticker, Type: journal.CancelCommand, Cancel: ev.CancelReq}, true
	default:
		slog.Warn("unsupported event type", "type", ev.Type)
		return journal.Command{}, false
	}
}

func (w *BookWorker) applyCommand(command journal.Command) error {
	if err := journal.Validate(command); err != nil {
		return err
	}
	if command.RecordedAt.IsZero() {
		return fmt.Errorf("%w: recorded time is required", journal.ErrInvalidCommand)
	}

	switch command.Type {
	case journal.CreateCommand:
		order := CreateOrderAt(*command.Create, command.RecordedAt)
		originalAmount := order.Amount
		logOrderReceived(&order)
		result := Match(w.OrderBook, &order)
		if err := w.persistMatchLogs(result.Logs); err != nil {
			return err
		}
		if result.Residual != nil {
			w.OrderBook.AddOrder(result.Residual)
			reason := "no_match"
			if result.Residual.Amount < originalAmount {
				reason = "partial_fill"
			}
			logOrderResting(result.Residual, reason)
		}
	case journal.AmendCommand:
		updated := w.OrderBook.EditOrderAt(*command.Amend, command.RecordedAt)
		if updated == nil {
			return nil
		}
		originalAmount := updated.Amount
		result := Match(w.OrderBook, updated)
		if err := w.persistMatchLogs(result.Logs); err != nil {
			return err
		}
		if result.Residual != nil {
			w.OrderBook.AddOrder(result.Residual)
			reason := "no_match"
			if result.Residual.Amount < originalAmount {
				reason = "partial_fill"
			}
			logOrderResting(result.Residual, reason)
		}
	case journal.CancelCommand:
		w.OrderBook.RemoveOrder(command.Cancel.OrderID)
	}
	return nil
}

func (w *BookWorker) persistMatchLogs(logs []matchlog.MatchLog) error {
	if len(logs) == 0 {
		return nil
	}
	if w.persistenceOut == nil {
		return matchlog.ErrStoreUnavailable
	}

	request := matchlog.NewPersistenceRequest(logs)
	w.persistenceOut <- request
	return request.Wait()
}

func (w *BookWorker) halt(err error) {
	w.state.Halt(err)
	slog.Error("matching engine halted", "ticker", w.ticker, "error", err)
}

func (w *BookWorker) Done() <-chan struct{} {
	return w.done
}

func (w *BookWorker) Err() error {
	return w.state.Err()
}

func isCommandEvent(eventType EventType) bool {
	return eventType == NewOrder || eventType == EditOrder || eventType == CancelOrder
}
