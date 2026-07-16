package engine

import "github.com/nogie-dev/clob-trading/internal/models"

type EventType int

const (
	NewOrder EventType = iota
	CancelOrder
	EditOrder
	snapshotOrderBook
)

type Event struct {
	Type   EventType
	Ticker string

	NewOrder  *models.CreateOrderRequest
	CancelReq *models.CancelOrderRequest
	EditReq   *models.EditOrderRequest

	snapshotDepth  int
	snapshotResult chan<- OrderBookSnapshot
}
