package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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

	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
