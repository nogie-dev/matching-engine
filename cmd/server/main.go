package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/nogie-dev/clob-trading/internal/api"
	"github.com/nogie-dev/clob-trading/internal/config"
	"github.com/nogie-dev/clob-trading/internal/engine"
)

func main() {
	configPath := flag.String("config", "config/default.json", "path to JSON config file")
	address := flag.String("addr", ":8080", "HTTP listen address")
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

	server := &http.Server{
		Addr:              *address,
		Handler:           api.NewHandler(r),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("internal API listening on %s", *address)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
