package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/lsp"
)

var (
	lspMachineConfig string
	lspProjectConfig string
	lspCacheDir      string
)

func init() {
	cmd := newLSPCmd()
	rootCmd.AddCommand(cmd)
}

func newLSPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lsp",
		Short: "Start the Language Server Protocol server",
		Long: `Start gavel in LSP mode to provide real-time code analysis in your editor.

The LSP server listens on stdin/stdout and provides diagnostics as you edit files.
Configuration is loaded from tiered sources (system → machine → project).`,
		RunE: runLSP,
	}

	cmd.Flags().StringVar(&lspMachineConfig, "machine-config", "", "Machine-level config file (default: $HOME/.config/gavel/policies.yaml)")
	cmd.Flags().StringVar(&lspProjectConfig, "project-config", ".gavel/policies.yaml", "Project-level config file")
	cmd.Flags().StringVar(&lspCacheDir, "cache-dir", "", "Cache directory (default: $HOME/.cache/gavel)")

	return cmd
}

func runLSP(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Set defaults for config paths
	if lspMachineConfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		lspMachineConfig = filepath.Join(home, ".config", "gavel", "policies.yaml")
	}

	// Set default cache directory
	if lspCacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		lspCacheDir = filepath.Join(home, ".cache", "gavel")
	}

	// Load tiered configuration
	cfg, err := config.LoadTiered(lspMachineConfig, lspProjectConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create BAML client
	client := analyzer.NewBAMLLiveClient(cfg.Provider)

	// Create analyzer wrapper with cache
	wrapper := lsp.NewAnalyzerWrapper(client, cfg)

	// Initialize cache if cache directory is set
	if lspCacheDir != "" {
		cacheManager := cache.NewLocalCache(lspCacheDir)
		wrapper = wrapper.WithCache(cacheManager)
	}

	// Create LSP server
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	server := lsp.NewServer(reader, writer, wrapper.Analyze)

	// Run server
	if err := server.Run(ctx); err != nil {
		return fmt.Errorf("LSP server error: %w", err)
	}

	return nil
}
