package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
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

	// Protect the LSP protocol stream from rogue stdout writes.
	// BAML is a C library (CGO) that writes debug output to fd 1 (stdout)
	// directly, bypassing Go's os.Stdout. We must redirect at the fd level:
	// 1. Duplicate fd 1 so the LSP server has exclusive access to real stdout
	// 2. Redirect fd 1 → fd 2 (stderr) so C libraries' stdout goes to stderr
	lspFD, err := syscall.Dup(1)
	if err != nil {
		return fmt.Errorf("duplicating stdout: %w", err)
	}
	if err := syscall.Dup2(2, 1); err != nil {
		return fmt.Errorf("redirecting stdout to stderr: %w", err)
	}
	lspOut := os.NewFile(uintptr(lspFD), "lsp-stdout")
	defer lspOut.Close()
	os.Stdout = os.Stderr // also redirect Go-level writes

	// Create LSP server with configuration
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(lspOut)

	server := lsp.NewServerWithConfig(reader, writer, wrapper.Analyze, serverConfig)

	// Set cache manager on server for commands
	if cacheManager != nil {
		server.SetCacheManager(cacheManager)
	}

	// Wire progressive analysis via TieredAnalyzer
	tieredAnalyzer := analyzer.NewTieredAnalyzer(client)

	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, cfg.Persona)
	if err != nil {
		return fmt.Errorf("getting persona prompt: %w", err)
	}

	server.SetProgressiveAnalyze(func(ctx context.Context, path, content string) <-chan lsp.ProgressiveResult {
		art := input.Artifact{Path: path, Content: content, Kind: input.KindFile}
		tieredCh := tieredAnalyzer.AnalyzeProgressive(ctx, []input.Artifact{art}, cfg.Policies, personaPrompt)

		resultCh := make(chan lsp.ProgressiveResult, 3)
		go func() {
			defer close(resultCh)
			for tr := range tieredCh {
				resultCh <- lsp.ProgressiveResult{
					Tier:    tr.Tier.String(),
					Results: tr.Results,
					Error:   tr.Error,
				}
			}
		}()
		return resultCh
	})

	// Run server
	if err := server.Run(ctx); err != nil {
		return fmt.Errorf("LSP server error: %w", err)
	}

	return nil
}
