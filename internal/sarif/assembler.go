package sarif

// Assemble creates a SARIF log from analysis results, deduplicating overlapping findings.
func Assemble(results []Result, rules []ReportingDescriptor, inputScope string) *Log {
	deduped := dedup(results)

	log := NewLog("gavel", "0.1.0")
	log.Runs[0].Tool.Driver.Rules = rules
	log.Runs[0].Results = deduped
	log.Runs[0].Properties = map[string]interface{}{
		"gavel/inputScope": inputScope,
	}

	return log
}

func dedup(results []Result) []Result {
	type key struct {
		ruleID string
		uri    string
	}

	best := make(map[key]Result)
	for _, r := range results {
		uri := ""
		if len(r.Locations) > 0 {
			uri = r.Locations[0].PhysicalLocation.ArtifactLocation.URI
		}
		k := key{ruleID: r.RuleID, uri: uri}

		existing, ok := best[k]
		if !ok {
			best[k] = r
			continue
		}

		if len(r.Locations) > 0 && len(existing.Locations) > 0 {
			rRegion := r.Locations[0].PhysicalLocation.Region
			eRegion := existing.Locations[0].PhysicalLocation.Region
			if regionsOverlap(rRegion, eRegion) {
				if confidence(r) > confidence(existing) {
					best[k] = r
				}
				continue
			}
		}

		// Non-overlapping same rule+file: keep both
		for i := 1; ; i++ {
			newKey := key{ruleID: r.RuleID + string(rune(i)), uri: uri}
			if _, exists := best[newKey]; !exists {
				best[newKey] = r
				break
			}
		}
	}

	out := make([]Result, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	return out
}

func regionsOverlap(a, b Region) bool {
	return a.StartLine <= b.EndLine && b.StartLine <= a.EndLine
}

func confidence(r Result) float64 {
	if r.Properties == nil {
		return 0
	}
	if c, ok := r.Properties["gavel/confidence"].(float64); ok {
		return c
	}
	return 0
}
