package engine

import (
	"log"
	"sync"
)

type Matcher struct {
	mu    sync.RWMutex
	books map[string]*OrderBook
}

func (m *Matcher) GetOrderBook(ticker string) *OrderBook {
	if _, ok := m.books[ticker]; !ok {
		log.Fatal("Not Exist OrderBook")
		return nil
	}
	return m.books[ticker]
}
