package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chris-regnier/gavel/internal/calibration/server"
)

func main() {
	addr := flag.String("addr", ":8090", "Listen address")
	dbPath := flag.String("db", "calibration.db", "SQLite database path")
	flag.Parse()

	apiKey := os.Getenv("CALIBRATION_API_KEY")
	if apiKey == "" {
		log.Fatal("CALIBRATION_API_KEY environment variable required")
	}

	store, err := server.NewSQLiteStore(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("closing database: %v", err)
		}
	}()

	handler := server.NewAPIServer(store, apiKey)
	srv := &http.Server{
		Addr:         *addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("Calibration server listening on %s\n", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-done
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
