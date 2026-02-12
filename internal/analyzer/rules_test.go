package analyzer

import (
	"testing"
)

func TestDefaultRules_Count(t *testing.T) {
	rules := DefaultRules()
	
	if len(rules) < 10 {
		t.Errorf("expected at least 10 default rules, got %d", len(rules))
	}
}

func TestDefaultRules_HasCWEReferences(t *testing.T) {
	rules := DefaultRules()
	
	var withCWE int
	for _, r := range rules {
		if len(r.CWE) > 0 {
			withCWE++
		}
	}
	
	// Most rules should have CWE references
	if withCWE < len(rules)/2 {
		t.Errorf("expected at least half the rules to have CWE references, got %d/%d", withCWE, len(rules))
	}
}

func TestDefaultRules_Categories(t *testing.T) {
	rules := DefaultRules()
	
	categories := make(map[RuleCategory]int)
	for _, r := range rules {
		categories[r.Category]++
	}
	
	if categories[CategorySecurity] == 0 {
		t.Error("expected at least one security rule")
	}
	if categories[CategoryReliability] == 0 {
		t.Error("expected at least one reliability rule")
	}
	if categories[CategoryMaintainability] == 0 {
		t.Error("expected at least one maintainability rule")
	}
	
	t.Logf("Rule categories: security=%d, reliability=%d, maintainability=%d",
		categories[CategorySecurity], categories[CategoryReliability], categories[CategoryMaintainability])
}

func TestSecurityRules(t *testing.T) {
	rules := SecurityRules()
	
	if len(rules) == 0 {
		t.Fatal("expected at least one security rule")
	}
	
	for _, r := range rules {
		if r.Category != CategorySecurity {
			t.Errorf("rule %s has category %s, expected security", r.ID, r.Category)
		}
	}
}

func TestReliabilityRules(t *testing.T) {
	rules := ReliabilityRules()
	
	if len(rules) == 0 {
		t.Fatal("expected at least one reliability rule")
	}
	
	for _, r := range rules {
		if r.Category != CategoryReliability {
			t.Errorf("rule %s has category %s, expected reliability", r.ID, r.Category)
		}
	}
}

func TestRulesByCWE(t *testing.T) {
	rules := DefaultRules()
	
	// SQL injection should be found
	sqlRules := RulesByCWE(rules, "CWE-89")
	if len(sqlRules) == 0 {
		t.Error("expected to find rules for CWE-89 (SQL Injection)")
	}
	
	// Hardcoded credentials
	credRules := RulesByCWE(rules, "CWE-798")
	if len(credRules) == 0 {
		t.Error("expected to find rules for CWE-798 (Hardcoded Credentials)")
	}
}

func TestPatternRule_Matching(t *testing.T) {
	rules := DefaultRules()
	
	testCases := []struct {
		name    string
		ruleID  string
		code    string
		matches bool
	}{
		{
			name:    "hardcoded credential pattern 1",
			ruleID:  "S2068",
			code:    `passwd = "test_value_12345678"`,
			matches: true,
		},
		{
			name:    "hardcoded credential pattern 2",
			ruleID:  "S2068",
			code:    `private_key = "test_key_abcdefghij"`,
			matches: true,
		},
		{
			name:    "safe credential from env",
			ruleID:  "S2068",
			code:    `cred := os.Getenv("MY_CREDENTIAL")`,
			matches: false,
		},
		{
			name:    "ignored error",
			ruleID:  "S1086",
			code:    `data, _ := getData()`,
			matches: true,
		},
		{
			name:    "handled error",
			ruleID:  "S1086",
			code:    `data, err := getData()`,
			matches: false,
		},
		{
			name:    "TODO comment",
			ruleID:  "S1135",
			code:    `// TODO: fix this later`,
			matches: true,
		},
		{
			name:    "FIXME comment",
			ruleID:  "S1135",
			code:    `// FIXME: broken`,
			matches: true,
		},
		{
			name:    "insecure TLS",
			ruleID:  "S4830",
			code:    `InsecureSkipVerify: true`,
			matches: true,
		},
		{
			name:    "secure TLS",
			ruleID:  "S4830",
			code:    `InsecureSkipVerify: false`,
			matches: false,
		},
		{
			name:    "weak crypto MD5",
			ruleID:  "S4426",
			code:    `hash := md5.Sum(data)`,
			matches: true,
		},
		{
			name:    "weak crypto SHA1",
			ruleID:  "S4426",
			code:    `hash := sha1.Sum(data)`,
			matches: true,
		},
		{
			name:    "strong crypto SHA256",
			ruleID:  "S4426",
			code:    `hash := sha256.Sum256(data)`,
			matches: false,
		},
		{
			name:    "error wrapping with %s",
			ruleID:  "G601",
			code:    `fmt.Errorf("failed: %s", err)`,
			matches: true,
		},
		{
			name:    "error wrapping with %w",
			ruleID:  "G601",
			code:    `fmt.Errorf("failed: %w", err)`,
			matches: false,
		},
	}
	
	// Build rule map
	ruleMap := make(map[string]PatternRule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule, ok := ruleMap[tc.ruleID]
			if !ok {
				t.Fatalf("rule %s not found", tc.ruleID)
			}
			
			matched := rule.Pattern.MatchString(tc.code)
			if matched != tc.matches {
				t.Errorf("rule %s on %q: expected match=%v, got match=%v", tc.ruleID, tc.code, tc.matches, matched)
			}
		})
	}
}

func TestPatternRule_AllHaveRequiredFields(t *testing.T) {
	rules := DefaultRules()
	
	for _, r := range rules {
		if r.ID == "" {
			t.Errorf("rule missing ID: %+v", r)
		}
		if r.Pattern == nil {
			t.Errorf("rule %s missing Pattern", r.ID)
		}
		if r.Level == "" {
			t.Errorf("rule %s missing Level", r.ID)
		}
		if r.Message == "" {
			t.Errorf("rule %s missing Message", r.ID)
		}
		if r.Explanation == "" {
			t.Errorf("rule %s missing Explanation", r.ID)
		}
		if r.Confidence <= 0 || r.Confidence > 1 {
			t.Errorf("rule %s has invalid Confidence: %f", r.ID, r.Confidence)
		}
		if r.Source == "" {
			t.Errorf("rule %s missing Source", r.ID)
		}
	}
}

func TestPatternRule_UniqueIDs(t *testing.T) {
	rules := DefaultRules()
	
	seen := make(map[string]bool)
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func BenchmarkDefaultRules(b *testing.B) {
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = DefaultRules()
	}
}

func BenchmarkPatternMatching_AllRules(b *testing.B) {
	rules := DefaultRules()
	code := `package main

import (
	"fmt"
	"crypto/md5"
)

func process() error {
	data, _ := getData()
	passwd = "test_value_12345678"
	// TODO: fix this
	hash := md5.Sum([]byte(data))
	fmt.Println(hash)
	return nil
}
`
	
	b.ReportAllocs()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		for _, rule := range rules {
			_ = rule.Pattern.FindAllStringIndex(code, -1)
		}
	}
}
