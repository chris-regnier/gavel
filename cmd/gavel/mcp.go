package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/config"
	gavelmcp "github.com/chris-regnier/gavel/internal/mcp"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/store"
)

var (
	mcpMachineConfig string
	mcpProjectConfig string
	mcpOutputDir     string
	mcpRegoDir       string
	mcpRulesDir      string
)

func init() {
	cmd := newMCPCmd()
	rootCmd.AddCommand(cmd)
}

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start the Model Context Protocol server",
		Long: `Start gavel as an MCP server to expose code analysis capabilities to AI agents.

The MCP server communicates over stdin/stdout using the Model Context Protocol,
allowing AI assistants like Claude to analyze code, evaluate results, and query
findings programmatically.

Tools provided:
  analyze_file       Analyze a single source file
  analyze_directory  Analyze all files in a directory
  judge              Evaluate analysis results with Rego policies
  list_results       List stored analysis results
  get_result         Get full SARIF output for a result

Resources:
  gavel://policies       Current policy configuration
  gavel://results/{id}   SARIF output for a specific result

Prompts:
  code-review            Code quality review workflow
  security-audit         Security vulnerability audit
  architecture-review    Architecture analysis workflow`,
		RunE: runMCP,
	}

	cmd.Flags().StringVar(&mcpMachineConfig, "machine-config", "", "Machine-level config file (default: $HOME/.config/gavel/policies.yaml)")
	cmd.Flags().StringVar(&mcpProjectConfig, "project-config", ".gavel/policies.yaml", "Project-level config file")
	cmd.Flags().StringVar(&mcpOutputDir, "output", ".gavel/results", "Output directory for results")
	cmd.Flags().StringVar(&mcpRegoDir, "rego-dir", "", "Directory containing custom Rego policies (default: embedded policy)")
	cmd.Flags().StringVar(&mcpRulesDir, "rules-dir", "", "Directory containing custom rule YAML files (default: sibling 'rules/' directory of --project-config)")

	return cmd
}

func runMCP(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Set defaults for config paths
	if mcpMachineConfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		mcpMachineConfig = filepath.Join(home, ".config", "gavel", "policies.yaml")
	}

	// Load tiered configuration
	cfg, err := config.LoadTiered(mcpMachineConfig, mcpProjectConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create file store
	fs := store.NewFileStore(mcpOutputDir)

	// Load rules (embedded defaults + user overrides + project overrides),
	// mirroring the CLI's tier-merging behavior.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	userRulesDir := filepath.Join(home, ".config", "gavel", "rules")

	projectRulesDir := mcpRulesDir
	if projectRulesDir == "" {
		projectRulesDir = filepath.Join(filepath.Dir(mcpProjectConfig), "rules")
	}

	loadedRules, err := rules.LoadRules(userRulesDir, projectRulesDir)
	if err != nil {
		return fmt.Errorf("loading rules: %w", err)
	}

	// Create MCP server
	mcpServer := gavelmcp.NewMCPServer(gavelmcp.ServerConfig{
		Config:  cfg,
		Store:   fs,
		RegoDir: mcpRegoDir,
		Rules:   loadedRules,
	})

	// Serve over stdio
	stdioServer := server.NewStdioServer(mcpServer)
	if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		return fmt.Errorf("MCP server error: %w", err)
	}

	return nil
}
