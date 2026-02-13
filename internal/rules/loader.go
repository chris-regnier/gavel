package rules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadRules(userDir, projectDir string) ([]Rule, error) {
	defaults, err := DefaultRules()
	if err != nil {
		return nil, fmt.Errorf("loading default rules: %w", err)
	}

	merged := indexByID(defaults)

	userRules, err := loadDir(userDir)
	if err != nil {
		return nil, fmt.Errorf("loading user rules from %s: %w", userDir, err)
	}
	for _, r := range userRules {
		merged[r.ID] = r
	}

	projectRules, err := loadDir(projectDir)
	if err != nil {
		return nil, fmt.Errorf("loading project rules from %s: %w", projectDir, err)
	}
	for _, r := range projectRules {
		merged[r.ID] = r
	}

	result := make([]Rule, 0, len(merged))
	for _, r := range merged {
		result = append(result, r)
	}
	return result, nil
}

func loadDir(dir string) ([]Rule, error) {
	if dir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var allRules []Rule
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		rf, err := ParseRuleFile(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		allRules = append(allRules, rf.Rules...)
	}
	return allRules, nil
}

func indexByID(rules []Rule) map[string]Rule {
	m := make(map[string]Rule, len(rules))
	for _, r := range rules {
		m[r.ID] = r
	}
	return m
}
