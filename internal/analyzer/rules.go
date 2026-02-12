package analyzer

import (
	"regexp"
)

// RuleCategory represents the category of a security/quality rule
type RuleCategory string

const (
	CategorySecurity    RuleCategory = "security"
	CategoryReliability RuleCategory = "reliability"
	CategoryMaintainability RuleCategory = "maintainability"
)

// RuleSource represents the source/standard for a rule
type RuleSource string

const (
	SourceCWE       RuleSource = "CWE"       // Common Weakness Enumeration
	SourceOWASP     RuleSource = "OWASP"     // OWASP Top 10 / Secure Coding
	SourceSonarQube RuleSource = "SonarQube" // SonarQube rules
	SourceCustom    RuleSource = "Custom"    // Custom rules
)

// PatternRule defines a regex-based instant check with standard references
type PatternRule struct {
	// Identification
	ID          string       // Unique rule identifier (e.g., "S1234" for SonarQube style)
	Name        string       // Human-readable name
	Category    RuleCategory // Rule category
	
	// Detection
	Pattern     *regexp.Regexp // Regex pattern to match
	Languages   []string       // Applicable languages (empty = all)
	
	// Severity
	Level       string  // SARIF level: error, warning, note
	Confidence  float64 // 0.0 to 1.0
	
	// Documentation
	Message     string // Short description shown inline
	Explanation string // Detailed explanation
	Remediation string // How to fix the issue
	
	// Standard References
	Source      RuleSource // Primary source (CWE, OWASP, etc.)
	CWE         []string   // CWE IDs (e.g., "CWE-89", "CWE-79")
	OWASP       []string   // OWASP categories (e.g., "A03:2021")
	References  []string   // URLs to documentation
}

// DefaultRules returns the built-in rules based on industry standards
func DefaultRules() []PatternRule {
	return []PatternRule{
		// =============================================================================
		// SECURITY RULES
		// =============================================================================

		// CWE-259: Use of Hard-coded Password
		// CWE-798: Use of Hard-coded Credentials
		// OWASP A07:2021 - Identification and Authentication Failures
		{
			ID:       "S2068",
			Name:     "hardcoded-credentials",
			Category: CategorySecurity,
			Pattern:  regexp.MustCompile(`(?i)(password|passwd|pwd|secret|api_key|apikey|api_secret|auth_token|access_token|private_key)\s*[:=]\s*["'][^"']{4,}["']`),
			Level:    "error",
			Confidence: 0.85,
			Message:    "Hard-coded credentials detected",
			Explanation: "Credentials should not be hard-coded in source code. They can be extracted from compiled applications or source repositories, leading to unauthorized access.",
			Remediation: "Store credentials in environment variables, a secrets manager (e.g., HashiCorp Vault, AWS Secrets Manager), or configuration files excluded from version control.",
			Source:   SourceCWE,
			CWE:      []string{"CWE-259", "CWE-798"},
			OWASP:    []string{"A07:2021"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/798.html",
				"https://owasp.org/Top10/A07_2021-Identification_and_Authentication_Failures/",
			},
		},

		// CWE-89: SQL Injection
		// OWASP A03:2021 - Injection
		{
			ID:       "S3649",
			Name:     "sql-injection",
			Category: CategorySecurity,
			Pattern:  regexp.MustCompile(`(?i)(SELECT|INSERT|UPDATE|DELETE|EXEC|EXECUTE)\s+.*(\+\s*["']|\+\s*\w+\s*\+|fmt\.Sprintf.*%[sv])`),
			Level:    "error",
			Confidence: 0.75,
			Message:    "Possible SQL injection vulnerability",
			Explanation: "String concatenation or formatting in SQL queries can allow attackers to inject malicious SQL code, potentially exposing or modifying database contents.",
			Remediation: "Use parameterized queries or prepared statements. In Go, use database/sql with placeholder parameters (?, $1, etc.).",
			Source:   SourceCWE,
			CWE:      []string{"CWE-89"},
			OWASP:    []string{"A03:2021"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/89.html",
				"https://owasp.org/Top10/A03_2021-Injection/",
				"https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html",
			},
		},

		// CWE-78: OS Command Injection
		// OWASP A03:2021 - Injection
		{
			ID:       "S2076",
			Name:     "command-injection",
			Category: CategorySecurity,
			Pattern:  regexp.MustCompile(`exec\.(Command|CommandContext)\s*\([^)]*\+|os/exec.*\+.*["']`),
			Languages: []string{"go"},
			Level:    "error",
			Confidence: 0.7,
			Message:    "Possible command injection vulnerability",
			Explanation: "Constructing OS commands with user input can allow attackers to execute arbitrary commands on the system.",
			Remediation: "Avoid constructing commands from user input. If necessary, use allowlists to validate input and avoid shell interpretation.",
			Source:   SourceCWE,
			CWE:      []string{"CWE-78"},
			OWASP:    []string{"A03:2021"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/78.html",
				"https://owasp.org/Top10/A03_2021-Injection/",
			},
		},

		// CWE-22: Path Traversal
		// OWASP A01:2021 - Broken Access Control
		{
			ID:       "S2083",
			Name:     "path-traversal",
			Category: CategorySecurity,
			Pattern:  regexp.MustCompile(`(os\.(Open|Create|ReadFile|WriteFile)|ioutil\.(ReadFile|WriteFile)|filepath\.Join)\s*\([^)]*\+`),
			Languages: []string{"go"},
			Level:    "warning",
			Confidence: 0.65,
			Message:    "Possible path traversal vulnerability",
			Explanation: "Constructing file paths with user input may allow attackers to access files outside the intended directory using '../' sequences.",
			Remediation: "Use filepath.Clean() and verify the resulting path is within the expected directory. Consider using filepath.Rel() to check containment.",
			Source:   SourceCWE,
			CWE:      []string{"CWE-22"},
			OWASP:    []string{"A01:2021"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/22.html",
				"https://owasp.org/Top10/A01_2021-Broken_Access_Control/",
			},
		},

		// CWE-327: Use of Broken Crypto Algorithm
		// CWE-328: Use of Weak Hash
		{
			ID:       "S4426",
			Name:     "weak-crypto",
			Category: CategorySecurity,
			Pattern:  regexp.MustCompile(`(md5|sha1|des|rc4)\.(New|Sum)`),
			Languages: []string{"go"},
			Level:    "warning",
			Confidence: 0.9,
			Message:    "Use of weak cryptographic algorithm",
			Explanation: "MD5, SHA1, DES, and RC4 are considered cryptographically weak and should not be used for security-sensitive operations.",
			Remediation: "Use SHA-256 or SHA-3 for hashing, and AES-GCM or ChaCha20-Poly1305 for encryption.",
			Source:   SourceCWE,
			CWE:      []string{"CWE-327", "CWE-328"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/327.html",
				"https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html",
			},
		},

		// CWE-295: Improper Certificate Validation
		{
			ID:       "S4830",
			Name:     "insecure-tls",
			Category: CategorySecurity,
			Pattern:  regexp.MustCompile(`InsecureSkipVerify\s*:\s*true`),
			Languages: []string{"go"},
			Level:    "error",
			Confidence: 0.95,
			Message:    "TLS certificate verification disabled",
			Explanation: "Disabling TLS certificate verification allows man-in-the-middle attacks. This should never be used in production.",
			Remediation: "Remove InsecureSkipVerify or set it to false. Configure proper CA certificates if using custom PKI.",
			Source:   SourceCWE,
			CWE:      []string{"CWE-295"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/295.html",
			},
		},

		// =============================================================================
		// RELIABILITY RULES
		// =============================================================================

		// CWE-252: Unchecked Return Value
		// SonarQube: go:S1086
		{
			ID:       "S1086",
			Name:     "error-ignored",
			Category: CategoryReliability,
			Pattern:  regexp.MustCompile(`(?m)^\s*[a-zA-Z_][a-zA-Z0-9_]*\s*,\s*_\s*:?=`),
			Languages: []string{"go"},
			Level:    "warning",
			Confidence: 0.75,
			Message:    "Error return value is ignored",
			Explanation: "Ignoring error return values can lead to silent failures, making bugs difficult to diagnose and potentially causing data corruption or security issues.",
			Remediation: "Handle the error appropriately: log it, return it, or explicitly document why it can be safely ignored.",
			Source:   SourceSonarQube,
			CWE:      []string{"CWE-252"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/252.html",
				"https://rules.sonarsource.com/go/RSPEC-1086",
			},
		},

		// Empty error check
		{
			ID:       "S1068",
			Name:     "empty-error-check",
			Category: CategoryReliability,
			Pattern:  regexp.MustCompile(`if\s+err\s*!=\s*nil\s*\{\s*\}`),
			Languages: []string{"go"},
			Level:    "warning",
			Confidence: 0.9,
			Message:    "Empty error handling block",
			Explanation: "Checking for an error but not handling it defeats the purpose of error checking and hides potential issues.",
			Remediation: "Add appropriate error handling: log the error, return it, or take corrective action.",
			Source:   SourceSonarQube,
			CWE:      []string{"CWE-252"},
			References: []string{
				"https://rules.sonarsource.com/go/RSPEC-1068",
			},
		},

		// CWE-561: Dead Code
		{
			ID:       "S1144",
			Name:     "unreachable-code",
			Category: CategoryReliability,
			Pattern:  regexp.MustCompile(`(?m)^\s*(return|panic|os\.Exit)\s*\([^)]*\)\s*\n\s*[a-zA-Z]`),
			Languages: []string{"go"},
			Level:    "warning",
			Confidence: 0.85,
			Message:    "Unreachable code detected",
			Explanation: "Code after return, panic, or os.Exit statements will never execute.",
			Remediation: "Remove the unreachable code or restructure the logic.",
			Source:   SourceCWE,
			CWE:      []string{"CWE-561"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/561.html",
			},
		},

		// Defer in loop - potential resource leak
		{
			ID:       "S2259",
			Name:     "defer-in-loop",
			Category: CategoryReliability,
			Pattern:  regexp.MustCompile(`for\s+.*\{[^}]*defer\s+`),
			Languages: []string{"go"},
			Level:    "warning",
			Confidence: 0.8,
			Message:    "Defer statement inside a loop",
			Explanation: "Deferred calls inside loops don't execute until the function returns, potentially causing resource leaks or unexpected behavior.",
			Remediation: "Move the deferred operation to a separate function or handle cleanup explicitly within each iteration.",
			Source:   SourceSonarQube,
			CWE:      []string{"CWE-404"},
			References: []string{
				"https://cwe.mitre.org/data/definitions/404.html",
			},
		},

		// =============================================================================
		// MAINTAINABILITY RULES
		// =============================================================================

		// TODO/FIXME comments
		{
			ID:       "S1135",
			Name:     "todo-fixme",
			Category: CategoryMaintainability,
			Pattern:  regexp.MustCompile(`(?i)(TODO|FIXME|HACK|XXX|BUG)[\s:]+`),
			Level:    "note",
			Confidence: 1.0,
			Message:    "Track this TODO/FIXME comment",
			Explanation: "TODO and FIXME comments indicate incomplete work or known issues that should be addressed.",
			Remediation: "Create a ticket to track this work item and either complete it or remove the comment.",
			Source:   SourceSonarQube,
			References: []string{
				"https://rules.sonarsource.com/go/RSPEC-1135",
			},
		},

		// Commented-out code
		{
			ID:       "S125",
			Name:     "commented-code",
			Category: CategoryMaintainability,
			Pattern:  regexp.MustCompile(`(?m)^\s*//\s*(if|for|func|return|var|const|type|switch|select)\s+`),
			Level:    "note",
			Confidence: 0.7,
			Message:    "Remove this commented-out code",
			Explanation: "Commented-out code clutters the codebase and should be removed. Version control preserves history.",
			Remediation: "Delete the commented code. Use version control to retrieve old code if needed.",
			Source:   SourceSonarQube,
			References: []string{
				"https://rules.sonarsource.com/go/RSPEC-125",
			},
		},

		// Debug print statements
		{
			ID:       "S106",
			Name:     "debug-print",
			Category: CategoryMaintainability,
			Pattern:  regexp.MustCompile(`(?m)^\s*(fmt\.Print|log\.Print|println)\s*\(`),
			Languages: []string{"go"},
			Level:    "note",
			Confidence: 0.8,
			Message:    "Replace this debug statement with a logger",
			Explanation: "Debug print statements should be replaced with proper logging that can be configured per environment.",
			Remediation: "Use a structured logging library (e.g., slog, zap, zerolog) with appropriate log levels.",
			Source:   SourceSonarQube,
			References: []string{
				"https://rules.sonarsource.com/go/RSPEC-106",
			},
		},

		// Error wrapping with %s instead of %w
		{
			ID:       "G601",
			Name:     "error-wrap-verb",
			Category: CategoryMaintainability,
			Pattern:  regexp.MustCompile(`fmt\.Errorf\s*\([^)]*%s[^)]*,\s*err\s*\)`),
			Languages: []string{"go"},
			Level:    "note",
			Confidence: 0.8,
			Message:    "Use %w instead of %s to wrap errors",
			Explanation: "Using %w preserves the error chain, allowing callers to use errors.Is() and errors.As() for error inspection.",
			Remediation: "Replace %s with %w when wrapping errors with fmt.Errorf().",
			Source:   SourceCustom,
			References: []string{
				"https://go.dev/blog/go1.13-errors",
			},
		},

		// Magic numbers
		{
			ID:       "S109",
			Name:     "magic-number",
			Category: CategoryMaintainability,
			Pattern:  regexp.MustCompile(`(?m)^\s*(?:if|for|switch|case|return)\s+.*[^0-9][2-9]\d{2,}[^0-9]`),
			Level:    "note",
			Confidence: 0.5,
			Message:    "Extract this magic number into a constant",
			Explanation: "Magic numbers make code harder to understand and maintain. Named constants provide context.",
			Remediation: "Define a constant with a descriptive name and use it instead of the literal value.",
			Source:   SourceSonarQube,
			References: []string{
				"https://rules.sonarsource.com/go/RSPEC-109",
			},
		},
	}
}

// RulesByCategory returns rules filtered by category
func RulesByCategory(rules []PatternRule, category RuleCategory) []PatternRule {
	var filtered []PatternRule
	for _, r := range rules {
		if r.Category == category {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// RulesByCWE returns rules that reference a specific CWE
func RulesByCWE(rules []PatternRule, cweID string) []PatternRule {
	var filtered []PatternRule
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

// SecurityRules returns only security-related rules
func SecurityRules() []PatternRule {
	return RulesByCategory(DefaultRules(), CategorySecurity)
}

// ReliabilityRules returns only reliability-related rules
func ReliabilityRules() []PatternRule {
	return RulesByCategory(DefaultRules(), CategoryReliability)
}

// MaintainabilityRules returns only maintainability-related rules
func MaintainabilityRules() []PatternRule {
	return RulesByCategory(DefaultRules(), CategoryMaintainability)
}
