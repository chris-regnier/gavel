package rules

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

type RuleCategory string

const (
	CategorySecurity        RuleCategory = "security"
	CategoryReliability     RuleCategory = "reliability"
	CategoryMaintainability RuleCategory = "maintainability"
)

type RuleSource string

const (
	SourceCWE       RuleSource = "CWE"
	SourceOWASP     RuleSource = "OWASP"
	SourceSonarQube RuleSource = "SonarQube"
	SourceCustom    RuleSource = "Custom"
)

type Rule struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Category    RuleCategory `yaml:"category"`
	Pattern     *regexp.Regexp `yaml:"-"`
	RawPattern  string       `yaml:"pattern"`
	Languages   []string     `yaml:"languages,omitempty"`
	Level       string       `yaml:"level"`
	Confidence  float64      `yaml:"confidence"`
	Message     string       `yaml:"message"`
	Explanation string       `yaml:"explanation,omitempty"`
	Remediation string       `yaml:"remediation,omitempty"`
	Source      RuleSource   `yaml:"source,omitempty"`
	CWE         []string     `yaml:"cwe,omitempty"`
	OWASP       []string     `yaml:"owasp,omitempty"`
	References  []string     `yaml:"references,omitempty"`
}

type RuleFile struct {
	Rules []Rule `yaml:"rules"`
}

func ParseRuleFile(data []byte) (*RuleFile, error) {
	var rf RuleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parsing rule file: %w", err)
	}

	seen := make(map[string]bool)
	for i := range rf.Rules {
		r := &rf.Rules[i]
		if err := validateRule(r); err != nil {
			return nil, fmt.Errorf("rule %q (index %d): %w", r.ID, i, err)
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("duplicate rule ID %q", r.ID)
		}
		seen[r.ID] = true

		compiled, err := regexp.Compile(r.RawPattern)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid regex pattern: %w", r.ID, err)
		}
		r.Pattern = compiled
	}

	return &rf, nil
}

func validateRule(r *Rule) error {
	if r.ID == "" {
		return fmt.Errorf("missing required field: id")
	}
	if r.RawPattern == "" {
		return fmt.Errorf("missing required field: pattern")
	}
	if r.Level == "" {
		return fmt.Errorf("missing required field: level")
	}
	if r.Message == "" {
		return fmt.Errorf("missing required field: message")
	}
	if r.Confidence <= 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence must be in range (0, 1], got %v", r.Confidence)
	}
	return nil
}

func ByCategory(rules []Rule, category RuleCategory) []Rule {
	var filtered []Rule
	for _, r := range rules {
		if r.Category == category {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func ByCWE(rules []Rule, cweID string) []Rule {
	var filtered []Rule
	for _, r := range rules {
		for _, cwe := range r.CWE {
			if cwe == cweID {
				filtered = append(filtered, r)
				break
			}
		}
	}
	return filtered
}
