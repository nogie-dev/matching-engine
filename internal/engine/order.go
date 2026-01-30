package engine

import (
	"container/heap"
	"container/list"
	"fmt"
	"log"
	"time"

	"github.com/nogie-dev/clob-trading/internal/models"
	"github.com/nogie-dev/clob-trading/internal/util"
)

type OrderBook struct {
	Bids      map[float64]*util.PriceLevel
	Asks      map[float64]*util.PriceLevel
	bidLevels util.MaxPriceHeap
	askLevels util.MinPriceHeap
	Index     map[string]*list.Element
	Ticker    string
}

func NewOrderBook(ticker string) *OrderBook {
	ob := &OrderBook{
		Ticker: ticker,
		Bids:   make(map[float64]*util.PriceLevel),
		Asks:   make(map[float64]*util.PriceLevel),
		Index:  make(map[string]*list.Element),
	}
	heap.Init(&ob.bidLevels)
	heap.Init(&ob.askLevels)
	return ob
}

func (ob *OrderBook) side(order *models.MakerOrder) (map[float64]*util.PriceLevel, heap.Interface, bool) {
	switch order.Position {
	case models.Bid:
		return ob.Bids, &ob.bidLevels, true
	case models.Ask:
		return ob.Asks, &ob.askLevels, true
	default:
		return nil, nil, false
	}
}

func CreateOrder(req models.RequestOrder) models.MakerOrder {
	return models.MakerOrder{
		OrderID:   util.GenerateOrderID(req),
		Ticker:    req.Ticker,
		UserID:    req.UserID,
		OrderType: req.OrderType,
		Position:  req.Position,
		Price:     req.Price,
		Amount:    req.Amount,
		Status:    models.Pending,
		Timestamp: time.Now(),
		Nonce:     req.Nonce,
	}
}

func (ob *OrderBook) AddOrder(order *models.MakerOrder) {
	var levels map[float64]*util.PriceLevel
	var h heap.Interface
	switch order.Position {
	case models.Bid:
		levels, h = ob.Bids, &ob.bidLevels
	case models.Ask:
		levels, h = ob.Asks, &ob.askLevels
	default:
		return
	}

	lvl, ok := levels[order.Price]
	// 해당 호가에 존재하지 않으면 호가 생성
	if !ok {
		lvl = &util.PriceLevel{Price: order.Price, Queue: util.NewQueue()}
		levels[order.Price] = lvl
		heap.Push(h, lvl)
	}
	lvl.TotalAmount += order.Amount
	ob.Index[order.OrderID] = lvl.Queue.Push(order)
}

func (ob *OrderBook) level(order *models.MakerOrder) (*util.PriceLevel, map[float64]*util.PriceLevel, heap.Interface, bool) {
	levels, h, ok := ob.side(order)
	if !ok {
		log.Printf("Unsupported position: %v", order.Position)
		return nil, nil, nil, false
	}
	lvl, ok := levels[order.Price]
	if !ok || lvl == nil {
		log.Printf("Price level not found: price=%.4f", order.Price)
		return nil, nil, nil, false
	}
	return lvl, levels, h, true
}

func (ob *OrderBook) RemoveOrder(order *models.MakerOrder) {
	lvl, levels, h, ok := ob.level(order)
	if !ok {
		return
	}

	elem, ok := ob.Index[order.OrderID]
	if !ok || elem == nil {
		log.Printf("Order not found in index: id=%s", order.OrderID)
		return
	}

	removed := lvl.Queue.Remove(elem)
	delete(ob.Index, order.OrderID)

	var amt float64
	if mo, ok := removed.(*models.MakerOrder); ok && mo != nil {
		amt = mo.Amount
	} else {
		amt = order.Amount
	}
	lvl.TotalAmount -= amt

	// 큐에 주문이 없을 경우 삭제
	if lvl.Queue.Len() == 0 {
		if lvl.Index >= 0 && lvl.Index < h.Len() {
			heap.Remove(h, lvl.Index)
		}
		delete(levels, lvl.Price)
	}
}

// func (ob *OrderBook) EditOrder(order *models.MakerOrder) {
// 	lvl, levels, h, ok := ob.level(order)

// }

func (ob *OrderBook) PrintOrderBook() {
	bidHeap := append(util.MaxPriceHeap(nil), ob.bidLevels...)
	heap.Init(&bidHeap)
	for bidHeap.Len() > 0 {
		lvl := heap.Pop(&bidHeap).(*util.PriceLevel)
		fmt.Printf("BID price=%.4f total=%.4f\n", lvl.Price, lvl.TotalAmount)
	}

	askHeap := append(util.MinPriceHeap(nil), ob.askLevels...)
	heap.Init(&askHeap)
	for askHeap.Len() > 0 {
		lvl := heap.Pop(&askHeap).(*util.PriceLevel)
		fmt.Printf("ASK price=%.4f total=%.4f\n", lvl.Price, lvl.TotalAmount)
	}
}
