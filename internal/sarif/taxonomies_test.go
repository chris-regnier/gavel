package sarif

import (
	"testing"
)

func TestBuildTaxonomies_Empty(t *testing.T) {
	result := BuildTaxonomies(nil)
	if result != nil {
		t.Errorf("expected nil for empty input, got %+v", result)
	}

	result = BuildTaxonomies([]ReportingDescriptor{})
	if result != nil {
		t.Errorf("expected nil for no-relationship input, got %+v", result)
	}
}

func TestBuildTaxonomies_SingleCWE(t *testing.T) {
	rules := []ReportingDescriptor{{
		ID: "S001",
		Relationships: []Relationship{{
			Target: RelationshipTarget{
				ID:            "798",
				ToolComponent: &ToolComponentReference{Name: "CWE"},
			},
			Kinds: []string{"relevant"},
		}},
	}}

	result := BuildTaxonomies(rules)
	if len(result) != 1 {
		t.Fatalf("expected 1 taxonomy, got %d", len(result))
	}
	if result[0].Name != "CWE" {
		t.Errorf("name: got %q", result[0].Name)
	}
	if result[0].Organization != "MITRE" {
		t.Errorf("organization: got %q", result[0].Organization)
	}
	if len(result[0].Taxa) != 1 || result[0].Taxa[0].ID != "798" {
		t.Errorf("taxa: got %+v", result[0].Taxa)
	}
}

func TestBuildTaxonomies_MultipleCWEs(t *testing.T) {
	rules := []ReportingDescriptor{{
		ID: "S001",
		Relationships: []Relationship{
			{Target: RelationshipTarget{ID: "798", ToolComponent: &ToolComponentReference{Name: "CWE"}}},
			{Target: RelationshipTarget{ID: "259", ToolComponent: &ToolComponentReference{Name: "CWE"}}},
		},
	}}

	result := BuildTaxonomies(rules)
	if len(result) != 1 {
		t.Fatalf("expected 1 taxonomy, got %d", len(result))
	}
	if len(result[0].Taxa) != 2 {
		t.Fatalf("expected 2 taxa, got %d", len(result[0].Taxa))
	}
	if result[0].Taxa[0].ID != "259" || result[0].Taxa[1].ID != "798" {
		t.Errorf("taxa not sorted: %+v", result[0].Taxa)
	}
}

func TestBuildTaxonomies_DedupAcrossRules(t *testing.T) {
	rules := []ReportingDescriptor{
		{
			ID:            "S001",
			Relationships: []Relationship{{Target: RelationshipTarget{ID: "798", ToolComponent: &ToolComponentReference{Name: "CWE"}}}},
		},
		{
			ID:            "S002",
			Relationships: []Relationship{{Target: RelationshipTarget{ID: "798", ToolComponent: &ToolComponentReference{Name: "CWE"}}}},
		},
	}

	result := BuildTaxonomies(rules)
	if len(result) != 1 {
		t.Fatalf("expected 1 taxonomy, got %d", len(result))
	}
	if len(result[0].Taxa) != 1 {
		t.Errorf("expected dedup to 1 taxon, got %d", len(result[0].Taxa))
	}
}

func TestBuildTaxonomies_MixedCWEAndOWASP(t *testing.T) {
	rules := []ReportingDescriptor{{
		ID: "S001",
		Relationships: []Relationship{
			{Target: RelationshipTarget{ID: "798", ToolComponent: &ToolComponentReference{Name: "CWE"}}},
			{Target: RelationshipTarget{ID: "A07:2021", ToolComponent: &ToolComponentReference{Name: "OWASP"}}},
		},
	}}

	result := BuildTaxonomies(rules)
	if len(result) != 2 {
		t.Fatalf("expected 2 taxonomies, got %d", len(result))
	}
	if result[0].Name != "CWE" || result[1].Name != "OWASP" {
		t.Errorf("taxonomy order: got [%q, %q]", result[0].Name, result[1].Name)
	}
	if result[0].Organization != "MITRE" {
		t.Errorf("CWE org: got %q", result[0].Organization)
	}
	if result[1].Organization != "OWASP Foundation" {
		t.Errorf("OWASP org: got %q", result[1].Organization)
	}
}

func TestBuildTaxonomies_NoRelationships(t *testing.T) {
	rules := []ReportingDescriptor{
		{ID: "R001"},
		{ID: "R002"},
	}

	result := BuildTaxonomies(rules)
	if result != nil {
		t.Errorf("expected nil for rules with no relationships, got %+v", result)
	}
}

func TestBuildTaxonomies_NilToolComponent(t *testing.T) {
	rules := []ReportingDescriptor{{
		ID: "S001",
		Relationships: []Relationship{{
			Target: RelationshipTarget{ID: "798"},
		}},
	}}

	result := BuildTaxonomies(rules)
	if result != nil {
		t.Errorf("expected nil for nil toolComponent, got %+v", result)
	}
}
