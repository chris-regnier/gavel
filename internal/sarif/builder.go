package sarif

import "github.com/chris-regnier/gavel/internal/config"

// Assembler provides a builder pattern for constructing SARIF logs with cache metadata
type Assembler struct {
	results       []Result
	rules         []ReportingDescriptor
	inputScope    string
	cacheMetadata *CacheMetadata
}

// NewAssembler creates a new Assembler with default values
func NewAssembler() *Assembler {
	return &Assembler{
		results: []Result{},
		rules:   []ReportingDescriptor{},
	}
}

// WithCacheMetadata configures cache metadata for the assembler
func (a *Assembler) WithCacheMetadata(fileHash string, cfg *config.Config, bamlVersion string) *Assembler {
	policies := make(map[string]PolicyMetadata)
	for name, policy := range cfg.Policies {
		if policy.Enabled {
			policies[name] = PolicyMetadata{
				Instruction: policy.Instruction,
				Version:     "", // Policy version can be added later if needed
			}
		}
	}

	model := ""
	provider := cfg.Provider.Name
	if provider == "openrouter" {
		model = cfg.Provider.OpenRouter.Model
	} else if provider == "ollama" {
		model = cfg.Provider.Ollama.Model
	}

	a.cacheMetadata = &CacheMetadata{
		FileHash:    fileHash,
		Provider:    provider,
		Model:       model,
		BAMLVersion: bamlVersion,
		Policies:    policies,
	}
	return a
}

// AddResults adds SARIF results to the assembler
func (a *Assembler) AddResults(results []Result) *Assembler {
	a.results = append(a.results, results...)
	return a
}

// AddRules adds reporting descriptors (rules) to the assembler
func (a *Assembler) AddRules(rules []ReportingDescriptor) *Assembler {
	a.rules = append(a.rules, rules...)
	return a
}

// WithInputScope sets the input scope for the SARIF log
func (a *Assembler) WithInputScope(scope string) *Assembler {
	a.inputScope = scope
	return a
}

// Build constructs the final SARIF log with all configured metadata
func (a *Assembler) Build() *Log {
	// Deduplicate results
	deduped := dedup(a.results)

	// Add cache metadata to each result if configured
	if a.cacheMetadata != nil {
		cacheKey := a.cacheMetadata.ComputeCacheKey()

		analyzerMetadata := map[string]interface{}{
			"provider": a.cacheMetadata.Provider,
			"model":    a.cacheMetadata.Model,
			"policies": convertPoliciesForJSON(a.cacheMetadata.Policies),
		}

		for i := range deduped {
			if deduped[i].Properties == nil {
				deduped[i].Properties = make(map[string]interface{})
			}
			deduped[i].Properties["gavel/cache_key"] = cacheKey
			deduped[i].Properties["gavel/analyzer"] = analyzerMetadata
		}
	}

	// Create log
	log := NewLog("gavel", "0.1.0")
	log.Runs[0].Tool.Driver.Rules = a.rules
	log.Runs[0].Results = deduped

	if a.inputScope != "" {
		log.Runs[0].Properties = map[string]interface{}{
			"gavel/inputScope": a.inputScope,
		}
	}

	return log
}

// convertPoliciesForJSON converts PolicyMetadata map to a format suitable for JSON
func convertPoliciesForJSON(policies map[string]PolicyMetadata) map[string]interface{} {
	result := make(map[string]interface{})
	for name, policy := range policies {
		result[name] = map[string]interface{}{
			"instruction": policy.Instruction,
			"version":     policy.Version,
		}
	}
	return result
}
