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
	lspCacheServer   string
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
	cmd.Flags().StringVar(&lspCacheServer, "cache-server", "", "Remote cache server URL (e.g., https://gavel.company.com)")

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

	// Initialize cache manager (multi-tier if remote cache is configured)
	var cacheManager cache.CacheManager

	// Build local cache
	var localCache cache.CacheManager
	if lspCacheDir != "" {
		localCache = cache.NewLocalCache(lspCacheDir)
	}

	// Determine remote cache URL (flag overrides config)
	remoteCacheURL := lspCacheServer
	if remoteCacheURL == "" && cfg.RemoteCache.Enabled {
		remoteCacheURL = cfg.RemoteCache.URL
	}

	// Build multi-tier cache if remote is configured
	if remoteCacheURL != "" && localCache != nil {
		// Get auth token if configured
		var remoteOpts []cache.RemoteCacheOption
		token, err := cfg.RemoteCache.GetRemoteCacheToken()
		if err != nil {
			return fmt.Errorf("getting remote cache token: %w", err)
		}
		if token != "" {
			remoteOpts = append(remoteOpts, cache.WithToken(token))
		}

		remoteCache := cache.NewRemoteCache(remoteCacheURL, remoteOpts...)

		// Build multi-tier config from settings
		multiTierConfig := cache.MultiTierConfig{
			WriteToRemote:        cfg.RemoteCache.Strategy.WriteToRemote,
			ReadFromRemote:       cfg.RemoteCache.Strategy.ReadFromRemote,
			PreferLocal:          cfg.RemoteCache.Strategy.PreferLocal,
			WarmLocalOnRemoteHit: cfg.RemoteCache.Strategy.WarmLocalOnRemoteHit,
		}

		cacheManager = cache.NewMultiTierCache(localCache, remoteCache, multiTierConfig)
	} else if localCache != nil {
		cacheManager = localCache
	}

	if cacheManager != nil {
		wrapper = wrapper.WithCache(cacheManager)
	}

	// Build server configuration from LSP config
	serverConfig := lsp.ServerConfigFromLSPConfig(cfg.LSP)

	// Create LSP server with configuration
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	server := lsp.NewServerWithConfig(reader, writer, wrapper.Analyze, serverConfig)

	// Set cache manager on server for commands
	if cacheManager != nil {
		server.SetCacheManager(cacheManager)
	}

	// Run server
	if err := server.Run(ctx); err != nil {
		return fmt.Errorf("LSP server error: %w", err)
	}

	return nil
}
