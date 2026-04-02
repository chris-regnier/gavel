// cmd/gavel/serve.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/server"
	"github.com/chris-regnier/gavel/internal/service"
	"github.com/chris-regnier/gavel/internal/store"
)

var (
	flagServeAddr         string
	flagServeAuthKeys     string
	flagServeStoreDir     string
	flagServeRegoDir      string
	flagServeMaxConc      int
	flagServeReadTimeout  time.Duration
	flagServeWriteTimeout time.Duration
)

func init() {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Gavel HTTP API server",
		Long:  "Start an HTTP server exposing Gavel analysis and evaluation via REST and SSE streaming.",
		RunE:  runServe,
	}

	cmd.Flags().StringVar(&flagServeAddr, "addr", ":8080", "Listen address")
	cmd.Flags().StringVar(&flagServeAuthKeys, "auth-keys", "", "Path to API keys file (key:tenant-id per line)")
	cmd.Flags().StringVar(&flagServeStoreDir, "store-dir", ".gavel/results", "Result storage directory")
	cmd.Flags().StringVar(&flagServeRegoDir, "rego-dir", "", "Custom Rego policy directory")
	cmd.Flags().IntVar(&flagServeMaxConc, "max-concurrent", 10, "Max concurrent analysis jobs")
	cmd.Flags().DurationVar(&flagServeReadTimeout, "read-timeout", 30*time.Second, "HTTP read timeout")
	cmd.Flags().DurationVar(&flagServeWriteTimeout, "write-timeout", 5*time.Minute, "HTTP write timeout (long for SSE)")

	rootCmd.AddCommand(cmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load auth keys
	authKeys := map[string]string{}
	if flagServeAuthKeys != "" {
		var err error
		authKeys, err = loadAuthKeys(flagServeAuthKeys)
		if err != nil {
			return fmt.Errorf("loading auth keys: %w", err)
		}
	}

	// Create store
	fs := store.NewFileStore(flagServeStoreDir)

	// Create services
	analyzeSvc := service.NewAnalyzeService(fs)
	judgeSvc := service.NewJudgeService(fs, flagServeRegoDir)

	// Build router
	router := server.NewRouter(server.RouterConfig{
		AnalyzeService: analyzeSvc,
		JudgeService:   judgeSvc,
		Store:          fs,
		AuthKeys:       authKeys,
		MaxConcurrent:  flagServeMaxConc,
	})

	// Start server
	srv := server.New(router, server.Config{
		Addr:         flagServeAddr,
		ReadTimeout:  flagServeReadTimeout,
		WriteTimeout: flagServeWriteTimeout,
	})

	return srv.Start(ctx)
}

// loadAuthKeys reads a file with "key:tenant-id" lines.
func loadAuthKeys(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	keys := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid auth key line: %q (expected key:tenant-id)", line)
		}
		keys[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return keys, scanner.Err()
}
