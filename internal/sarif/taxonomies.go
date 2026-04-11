package sarif

import "sort"

// taxonomyOrgs maps well-known taxonomy names to their organizations.
var taxonomyOrgs = map[string]string{
	"CWE":   "MITRE",
	"OWASP": "OWASP Foundation",
}

// BuildTaxonomies walks the Relationships of each rule and returns the
// unique set of SARIF toolComponents referenced, each populated with its
// taxa. Taxa and taxonomies are sorted deterministically. Returns nil when
// no relationships reference a toolComponent.
func BuildTaxonomies(rules []ReportingDescriptor) []ToolComponent {
	groups := make(map[string]map[string]struct{})

	for _, rule := range rules {
		for _, rel := range rule.Relationships {
			if rel.Target.ToolComponent == nil {
				continue
			}
			name := rel.Target.ToolComponent.Name
			if _, ok := groups[name]; !ok {
				groups[name] = make(map[string]struct{})
			}
			groups[name][rel.Target.ID] = struct{}{}
		}
	}

	if len(groups) == 0 {
		return nil
	}

	var result []ToolComponent
	for name, ids := range groups {
		taxa := make([]Taxon, 0, len(ids))
		for id := range ids {
			taxa = append(taxa, Taxon{ID: id})
		}
		sort.Slice(taxa, func(i, j int) bool { return taxa[i].ID < taxa[j].ID })

		tc := ToolComponent{
			Name:         name,
			Organization: taxonomyOrgs[name],
			Taxa:         taxa,
		}
		result = append(result, tc)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
