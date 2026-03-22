package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/suppression"
)

var (
	flagSuppressFile   string
	flagSuppressReason string
	flagSuppressSource string
)

func init() {
	suppressCmd.Flags().StringVar(&flagSuppressFile, "file", "", "Restrict suppression to this file path")
	suppressCmd.Flags().StringVar(&flagSuppressReason, "reason", "", "Reason for suppression (required)")
	suppressCmd.MarkFlagRequired("reason")

	unsuppressCmd.Flags().StringVar(&flagSuppressFile, "file", "", "Remove file-specific suppression only")

	suppressionsCmd.Flags().StringVar(&flagSuppressSource, "source", "", "Filter by source prefix (e.g., \"mcp:\")")

	rootCmd.AddCommand(suppressCmd)
	rootCmd.AddCommand(unsuppressCmd)
	rootCmd.AddCommand(suppressionsCmd)
}

var suppressCmd = &cobra.Command{
	Use:   "suppress <rule-id>",
	Short: "Suppress a finding rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		return runSuppress(dir, args[0], flagSuppressFile, flagSuppressReason)
	},
}

var unsuppressCmd = &cobra.Command{
	Use:   "unsuppress <rule-id>",
	Short: "Remove a finding suppression",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		return runUnsuppress(dir, args[0], flagSuppressFile)
	},
}

var suppressionsCmd = &cobra.Command{
	Use:   "suppressions",
	Short: "List active suppressions",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		return runListSuppressions(dir, flagSuppressSource)
	},
}

func runSuppress(projectDir, ruleID, file, reason string) error {
	supps, err := suppression.Load(projectDir)
	if err != nil {
		return fmt.Errorf("loading suppressions: %w", err)
	}

	normalizedFile := ""
	if file != "" {
		normalizedFile = suppression.NormalizePath(file)
	}

	source := "cli:user:" + currentUsername()

	for i := range supps {
		existingFile := supps[i].File
		if existingFile != "" {
			existingFile = suppression.NormalizePath(existingFile)
		}
		if supps[i].RuleID == ruleID && existingFile == normalizedFile {
			supps[i].Reason = reason
			supps[i].Created = time.Now().UTC().Truncate(time.Second)
			supps[i].Source = source
			if err := suppression.Save(projectDir, supps); err != nil {
				return fmt.Errorf("saving suppressions: %w", err)
			}
			slog.Info("updated suppression", "rule_id", ruleID, "file", normalizedFile)
			return nil
		}
	}

	entry := suppression.Suppression{
		RuleID:  ruleID,
		File:    normalizedFile,
		Reason:  reason,
		Created: time.Now().UTC().Truncate(time.Second),
		Source:  source,
	}
	supps = append(supps, entry)

	if err := suppression.Save(projectDir, supps); err != nil {
		return fmt.Errorf("saving suppressions: %w", err)
	}
	slog.Info("added suppression", "rule_id", ruleID, "file", normalizedFile)
	return nil
}

func runUnsuppress(projectDir, ruleID, file string) error {
	supps, err := suppression.Load(projectDir)
	if err != nil {
		return fmt.Errorf("loading suppressions: %w", err)
	}

	normalizedFile := ""
	if file != "" {
		normalizedFile = suppression.NormalizePath(file)
	}

	for i := range supps {
		existingFile := supps[i].File
		if existingFile != "" {
			existingFile = suppression.NormalizePath(existingFile)
		}
		if supps[i].RuleID == ruleID && existingFile == normalizedFile {
			supps = append(supps[:i], supps[i+1:]...)
			if err := suppression.Save(projectDir, supps); err != nil {
				return fmt.Errorf("saving suppressions: %w", err)
			}
			slog.Info("removed suppression", "rule_id", ruleID, "file", normalizedFile)
			return nil
		}
	}

	return fmt.Errorf("no suppression found for rule %s (file: %q)", ruleID, file)
}

func runListSuppressions(projectDir, sourceFilter string) error {
	supps, err := suppression.Load(projectDir)
	if err != nil {
		return fmt.Errorf("loading suppressions: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "RULE\tFILE\tSOURCE\tREASON")

	for _, s := range supps {
		if sourceFilter != "" && !strings.HasPrefix(s.Source, sourceFilter) {
			continue
		}
		file := "(all)"
		if s.File != "" {
			file = s.File
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.RuleID, file, s.Source, s.Reason)
	}
	w.Flush()
	return nil
}

func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}
