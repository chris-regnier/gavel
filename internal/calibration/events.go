package calibration

import (
	"path/filepath"
	"time"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// BuildEventsFromSARIF creates calibration events from a SARIF log.
//
// It returns one EventAnalysisCompleted event followed by one EventFindingCreated
// event per result in the first run of the log. Returns nil when the log contains
// no runs.
//
// Parameters:
//   - log:        the SARIF log produced by an analysis run.
//   - resultID:   the store result ID that identifies this analysis.
//   - persona:    the gavel persona used during analysis (e.g. "code-reviewer").
//   - provider:   the LLM provider name (e.g. "openrouter", "anthropic").
//   - model:      the LLM model name used for the analysis.
//   - shareCode:  reserved for future use; when true callers may include code
//                 snippets in payloads (currently unused).
func BuildEventsFromSARIF(log *sarif.Log, resultID, persona, provider, model string, shareCode bool) []Event {
	now := time.Now().UTC()

	if len(log.Runs) == 0 {
		return nil
	}
	run := log.Runs[0]

	// Collect unique rule IDs and file-extension types across all results so
	// the AnalysisPayload can report which rules and language types were seen.
	ruleIDs := map[string]bool{}
	fileTypes := map[string]bool{}
	for _, r := range run.Results {
		ruleIDs[r.RuleID] = true
		if len(r.Locations) > 0 {
			ext := filepath.Ext(r.Locations[0].PhysicalLocation.ArtifactLocation.URI)
			if ext != "" {
				fileTypes[ext] = true
			}
		}
	}

	ruleList := make([]string, 0, len(ruleIDs))
	for id := range ruleIDs {
		ruleList = append(ruleList, id)
	}
	ftList := make([]string, 0, len(fileTypes))
	for ft := range fileTypes {
		ftList = append(ftList, ft)
	}

	events := make([]Event, 0, 1+len(run.Results))

	// Emit the analysis-level summary event first.
	events = append(events, Event{
		Type:      EventAnalysisCompleted,
		Timestamp: now,
		Payload: AnalysisPayload{
			ResultID:     resultID,
			RuleIDs:      ruleList,
			FileTypes:    ftList,
			FindingCount: len(run.Results),
			Provider:     provider,
			Model:        model,
			Persona:      persona,
		},
	})

	// Emit one finding event per result.
	for _, r := range run.Results {
		fp := FindingPayload{
			ResultID: resultID,
			RuleID:   r.RuleID,
			Severity: r.Level,
			Message:  r.Message.Text,
		}

		// Extract model confidence from the gavel-specific SARIF property when
		// present. The property is stored as a float64 via JSON unmarshalling.
		if conf, ok := r.Properties["gavel/confidence"].(float64); ok {
			fp.Confidence = conf
		}

		// Populate file-location fields from the first location when available.
		if len(r.Locations) > 0 {
			loc := r.Locations[0].PhysicalLocation
			fp.FileType = filepath.Ext(loc.ArtifactLocation.URI)
			fp.StartLine = loc.Region.StartLine
			fp.EndLine = loc.Region.EndLine
		}

		events = append(events, Event{
			Type:      EventFindingCreated,
			Timestamp: now,
			Payload:   fp,
		})
	}

	return events
}
