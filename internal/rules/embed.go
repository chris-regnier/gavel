package rules

import _ "embed"

//go:embed default_rules.yaml
var defaultRulesYAML []byte

func DefaultRules() ([]Rule, error) {
	rf, err := ParseRuleFile(defaultRulesYAML)
	if err != nil {
		return nil, err
	}
	return rf.Rules, nil
}
