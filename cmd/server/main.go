package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nogie-dev/clob-trading/internal/api"
	"github.com/nogie-dev/clob-trading/internal/config"
	"github.com/nogie-dev/clob-trading/internal/engine"
	"github.com/nogie-dev/clob-trading/internal/matchlog"
	matchlogpostgres "github.com/nogie-dev/clob-trading/internal/matchlog/postgres"
)

const databaseURLEnv = "MATCHING_ENGINE_DATABASE_URL"

type engineRuntime struct {
	router         *engine.Router
	persistenceOut chan matchlog.PersistenceRequest
	writerDone     chan struct{}
}

func main() {
	configPath := flag.String("config", "config/default.json", "path to JSON config file")
	address := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	databaseURL, err := requiredDatabaseURL(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	if err := serve(cfg, *address, databaseURL); err != nil {
		log.Fatal(err)
	}
}

func requiredDatabaseURL(getenv func(string) string) (string, error) {
	url := strings.TrimSpace(getenv(databaseURLEnv))
	if url == "" {
		return "", fmt.Errorf("%s is required", databaseURLEnv)
	}
	return url, nil
}

func serve(cfg config.Config, address, databaseURL string) error {
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStartup()

	pool, err := pgxpool.New(startupCtx, databaseURL)
	if err != nil {
		return fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(startupCtx); err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}

	runtime, err := startEngine(cfg, matchlogpostgres.NewStore(pool))
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              address,
		Handler:           api.NewHandler(runtime.router),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	log.Printf("internal API listening on %s", address)
	var listenErr error
	select {
	case listenErr = <-serverErr:
	case <-signalCtx.Done():
	}

	httpShutdownCtx, cancelHTTPShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	httpShutdownErr := server.Shutdown(httpShutdownCtx)
	cancelHTTPShutdown()
	if httpShutdownErr != nil {
		httpShutdownErr = fmt.Errorf("shutdown HTTP server: %w", httpShutdownErr)
	}
	if listenErr == nil {
		listenErr = <-serverErr
	}
	engineShutdownErr := runtime.shutdown(context.Background())
	if listenErr != nil && errors.Is(listenErr, http.ErrServerClosed) {
		listenErr = nil
	} else if listenErr != nil {
		listenErr = fmt.Errorf("serve HTTP: %w", listenErr)
	}
	return errors.Join(httpShutdownErr, engineShutdownErr, listenErr)
}

func startEngine(cfg config.Config, store matchlog.Store) (*engineRuntime, error) {
	persistenceOut := make(chan matchlog.PersistenceRequest, cfg.Engine.MatchLogOutputBufferSize)
	writerDone := make(chan struct{})
	writer := matchlog.NewWriter(store)
	go func() {
		defer close(writerDone)
		writer.Run(context.Background(), persistenceOut)
	}()

	router := engine.NewRouter()
	worker := engine.NewBookWorkerWithOptions("BTC-USD", nil, engine.BookWorkerOptions{
		InputBufferSize: cfg.Engine.WorkerInputBufferSize,
		PersistenceOut:  persistenceOut,
	})
	if err := router.Register("BTC-USD", worker); err != nil {
		close(persistenceOut)
		<-writerDone
		return nil, fmt.Errorf("register BTC-USD worker: %w", err)
	}
	go worker.Run()

	return &engineRuntime{
		router:         router,
		persistenceOut: persistenceOut,
		writerDone:     writerDone,
	}, nil
}

func (r *engineRuntime) shutdown(ctx context.Context) error {
	if err := r.router.Shutdown(ctx); err != nil {
		return err
	}
	close(r.persistenceOut)
	select {
	case <-r.writerDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown match log writer: %w", ctx.Err())
	}
}
