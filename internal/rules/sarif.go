package rules

import (
	"fmt"
	"strings"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// ToSARIFDescriptor converts a Rule into a SARIF reportingDescriptor with
// help text and helpUri populated from the rule's remediation, CWE, OWASP,
// and reference metadata. This enables SARIF viewers (GitHub Code Scanning,
// VS Code) to render rich, actionable rule documentation alongside findings.
func (r Rule) ToSARIFDescriptor() sarif.ReportingDescriptor {
	d := sarif.ReportingDescriptor{
		ID:               r.ID,
		Name:             r.Name,
		ShortDescription: sarif.Message{Text: r.Message},
	}

	if r.Level != "" {
		d.DefaultConfig = &sarif.ReportingConfiguration{Level: r.Level}
	}

	if r.Explanation != "" {
		d.FullDescription = &sarif.Message{Text: r.Explanation}
	}

	if help := buildHelp(r); help != nil {
		d.Help = help
	}

	d.HelpURI = resolveHelpURI(r)

	return d
}

// buildHelp assembles a MultiformatMessage from remediation, CWE/OWASP,
// and reference links. Returns nil when no help content is available.
func buildHelp(r Rule) *sarif.MultiformatMessage {
	if r.Remediation == "" && len(r.CWE) == 0 && len(r.OWASP) == 0 && len(r.References) == 0 {
		return nil
	}

	var textParts []string
	var mdParts []string

	if r.Remediation != "" {
		textParts = append(textParts, r.Remediation)
		mdParts = append(mdParts, "**Remediation:** "+r.Remediation)
	}

	if len(r.CWE) > 0 {
		var cweLinks []string
		for _, id := range r.CWE {
			cweLinks = append(cweLinks, fmt.Sprintf("[%s](%s)", id, cweURL(id)))
		}
		textParts = append(textParts, "CWE: "+strings.Join(r.CWE, ", "))
		mdParts = append(mdParts, "**CWE:** "+strings.Join(cweLinks, ", "))
	}

	if len(r.OWASP) > 0 {
		joined := strings.Join(r.OWASP, ", ")
		textParts = append(textParts, "OWASP: "+joined)
		mdParts = append(mdParts, "**OWASP:** "+joined)
	}

	if len(r.References) > 0 {
		textLines := []string{"References:"}
		mdLines := []string{"**References:**"}
		for _, ref := range r.References {
			textLines = append(textLines, "- "+ref)
			mdLines = append(mdLines, "- "+ref)
		}
		textParts = append(textParts, strings.Join(textLines, "\n"))
		mdParts = append(mdParts, strings.Join(mdLines, "\n"))
	}

	return &sarif.MultiformatMessage{
		Text:     strings.Join(textParts, "\n\n"),
		Markdown: strings.Join(mdParts, "\n\n"),
	}
}

// resolveHelpURI picks a canonical documentation URL for the rule. It prefers
// the first entry in References (typically the CWE or OWASP page), then
// synthesizes a cwe.mitre.org URL if a CWE id is present, and otherwise
// returns an empty string.
func resolveHelpURI(r Rule) string {
	if len(r.References) > 0 {
		return r.References[0]
	}
	if len(r.CWE) > 0 {
		return cweURL(r.CWE[0])
	}
	return ""
}

// cweURL returns the canonical cwe.mitre.org URL for a CWE id like "CWE-798".
func cweURL(id string) string {
	num := strings.TrimPrefix(id, "CWE-")
	return "https://cwe.mitre.org/data/definitions/" + num + ".html"
}
