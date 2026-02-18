package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/output"
)

var (
	// Version information injected by goreleaser
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "gavel",
	Short:   "AI-powered code analysis with structured output",
	Version: version,
}

func init() {
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		quiet, _ := cmd.Flags().GetBool("quiet")
		verbose, _ := cmd.Flags().GetBool("verbose")
		debug, _ := cmd.Flags().GetBool("debug")
		logger := output.SetupLogger(quiet, verbose, debug, os.Stderr)
		slog.SetDefault(logger)
		return nil
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gavel %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built at: %s\n", date)
	},
}

func init() {
	// Add --persona flag globally (available to all subcommands)
	rootCmd.PersistentFlags().String(
		"persona",
		"",
		"Persona to use for analysis (code-reviewer, architect, security). Overrides config.",
	)

	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress all log output")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose (info-level) logging")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug-level logging")

	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
