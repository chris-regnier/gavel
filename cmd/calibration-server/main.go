package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

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
	defer store.Close()

	srv := server.NewAPIServer(store, apiKey)

	fmt.Printf("Calibration server listening on %s\n", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Fatal(err)
	}
}
