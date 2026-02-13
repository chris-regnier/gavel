package rules

import (
	"testing"
)

func TestDefaultRules_LoadsEmbedded(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("DefaultRules() returned error: %v", err)
	}
	if len(rules) < 10 {
		t.Fatalf("expected at least 10 rules, got %d", len(rules))
	}
}

func TestDefaultRules_HasAllCategories(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("DefaultRules() returned error: %v", err)
	}

	counts := map[RuleCategory]int{}
	for _, r := range rules {
		counts[r.Category]++
	}

	for _, cat := range []RuleCategory{CategorySecurity, CategoryReliability, CategoryMaintainability} {
		if counts[cat] == 0 {
			t.Errorf("expected at least 1 rule in category %q, got 0", cat)
		}
	}
}

func TestDefaultRules_UniqueIDs(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("DefaultRules() returned error: %v", err)
	}

	seen := make(map[string]bool)
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestDefaultRules_PatternsCompile(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("DefaultRules() returned error: %v", err)
	}

	for _, r := range rules {
		if r.Type == RuleTypeRegex && r.Pattern == nil {
			t.Errorf("regex rule %s has nil compiled pattern", r.ID)
		}
		if r.Type == RuleTypeAST && r.Pattern != nil {
			t.Errorf("ast rule %s should not have compiled pattern", r.ID)
		}
	}
}

func TestDefaultRules_ContainsASTRules(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("DefaultRules() returned error: %v", err)
	}

	astCount := 0
	for _, r := range rules {
		if r.Type == RuleTypeAST {
			astCount++
			if r.ASTCheck == "" {
				t.Errorf("AST rule %s missing ast_check", r.ID)
			}
		}
	}
	if astCount < 4 {
		t.Errorf("expected at least 4 AST rules, got %d", astCount)
	}
}

func TestDefaultRules_KnownRulesExist(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("DefaultRules() returned error: %v", err)
	}

	known := map[string]string{
		"S2068": "hardcoded-credentials",
		"S3649": "sql-injection",
		"S4830": "insecure-tls",
		"S1086": "error-ignored",
		"S1135": "todo-fixme",
	}

	ruleMap := make(map[string]Rule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	for id, name := range known {
		r, ok := ruleMap[id]
		if !ok {
			t.Errorf("expected rule %s to exist", id)
			continue
		}
		if r.Name != name {
			t.Errorf("rule %s: expected name %q, got %q", id, name, r.Name)
		}
	}
}

func TestDefaultRules_PatternMatching(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("DefaultRules() returned error: %v", err)
	}

	ruleMap := make(map[string]Rule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	tests := []struct {
		ruleID string
		input  string
	}{
		{"S2068", `password = "secret123"`},
		{"S4830", `InsecureSkipVerify: true`},
		{"S1135", `// TODO: fix this`},
		{"S4426", `md5.Sum(data)`},
	}

	for _, tc := range tests {
		t.Run(tc.ruleID, func(t *testing.T) {
			r, ok := ruleMap[tc.ruleID]
			if !ok {
				t.Fatalf("rule %s not found", tc.ruleID)
			}
			if !r.Pattern.MatchString(tc.input) {
				t.Errorf("rule %s pattern did not match %q", tc.ruleID, tc.input)
			}
		})
	}
}
