package main

import (
	"flag"
	"log"
	"time"

	"github.com/nogie-dev/clob-trading/internal/config"
	"github.com/nogie-dev/clob-trading/internal/engine"
	"github.com/nogie-dev/clob-trading/internal/testdata"
)

func main() {
	configPath := flag.String("config", "config/default.json", "path to JSON config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	r := engine.NewRouter()
	w := engine.NewBookWorkerWithOptions("BTC-USD", nil, engine.BookWorkerOptions{
		InputBufferSize: cfg.Engine.WorkerInputBufferSize,
	})
	go w.Run()

	if err := r.Register("BTC-USD", w); err != nil {
		log.Fatal(err)
	}

	for _, req := range testdata.SampleOrders {
		ev := engine.Event{
			Type:     engine.NewOrder,
			Ticker:   req.Ticker,
			NewOrder: &req,
		}
		if err := r.OrderRouter(ev); err != nil {
			log.Printf("route error: %v", err)
		}
	}

	// Give the worker a moment to drain the channel before printing.
	time.Sleep(50 * time.Millisecond)
	w.OrderBook.PrintOrderBook()
}
