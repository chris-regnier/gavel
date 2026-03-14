package calibration

import "time"

// EventType identifies the kind of calibration event.
type EventType string

const (
	EventAnalysisCompleted EventType = "analysis_completed"
	EventFindingCreated    EventType = "finding_created"
	EventFeedbackReceived  EventType = "feedback_received"
	EventOutcomeObserved   EventType = "outcome_observed"
)

// Valid reports whether t is a recognised event type.
func (t EventType) Valid() bool {
	switch t {
	case EventAnalysisCompleted, EventFindingCreated,
		EventFeedbackReceived, EventOutcomeObserved:
		return true
	}
	return false
}

// Event is a single calibration event uploaded by the CLI.
type Event struct {
	Type      EventType   `json:"type"`
	TeamID    string      `json:"team_id"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// FindingPayload is the payload for EventFindingCreated.
type FindingPayload struct {
	ResultID    string  `json:"result_id"`
	RuleID      string  `json:"rule_id"`
	Severity    string  `json:"severity"`
	Confidence  float64 `json:"confidence"`
	FileType    string  `json:"file_type"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Message     string  `json:"message"`
	Explanation string  `json:"explanation,omitempty"`
	CodeSnippet string  `json:"code_snippet,omitempty"`
}

// FeedbackPayload is the payload for EventFeedbackReceived.
type FeedbackPayload struct {
	ResultID     string `json:"result_id"`
	FindingIndex int    `json:"finding_index"`
	RuleID       string `json:"rule_id"`
	Verdict      string `json:"verdict"`
	Reason       string `json:"reason,omitempty"`
}

// OutcomePayload is the payload for EventOutcomeObserved.
type OutcomePayload struct {
	ResultID        string `json:"result_id"`
	FindingIndex    int    `json:"finding_index"`
	RuleID          string `json:"rule_id"`
	OutcomeType     string `json:"outcome_type"`
	TimeToResolveMs int64  `json:"time_to_resolve_ms,omitempty"`
}

// AnalysisPayload is the payload for EventAnalysisCompleted.
type AnalysisPayload struct {
	ResultID     string   `json:"result_id"`
	RuleIDs      []string `json:"rule_ids"`
	FileTypes    []string `json:"file_types"`
	FindingCount int      `json:"finding_count"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Persona      string   `json:"persona"`
}

// RuleProfile is a materialized per-team calibration profile for a single rule.
// It is derived by replaying FeedbackPayload and OutcomePayload events and
// updated incrementally on the hot path via UpdateProfileFromFeedback.
//
// Call Recalculate after mutating count fields to keep derived fields in sync.
type RuleProfile struct {
	// TeamID identifies the owning team.
	TeamID string `json:"team_id"`

	// RuleID is the rule this profile describes.
	RuleID string `json:"rule_id"`

	// TotalFindings is the cumulative number of findings emitted for this rule.
	TotalFindings int `json:"total_findings"`

	// UsefulCount is the number of findings rated as useful (true positive) via feedback.
	UsefulCount int `json:"useful_count"`

	// NoiseCount is the number of findings rated as noise (false positive) via feedback.
	NoiseCount int `json:"noise_count"`

	// WrongCount is the number of findings rated as wrong (incorrect diagnosis) via feedback.
	WrongCount int `json:"wrong_count"`

	// NoiseRate is NoiseCount / TotalFindings, or 0 if TotalFindings == 0.
	// Populated by Recalculate.
	NoiseRate float64 `json:"noise_rate"`

	// ConfCalibration is the difference between MeanUsefulConf and MeanNoiseConf.
	// A value near 0 indicates the model's confidence score does not discriminate
	// between useful and noisy findings for this rule.
	// Populated by Recalculate; 0 when confidence data is absent.
	ConfCalibration float64 `json:"conf_calibration"`

	// MeanNoiseConf is the mean model confidence across noise-rated findings.
	MeanNoiseConf float64 `json:"mean_noise_conf,omitempty"`

	// MeanUsefulConf is the mean model confidence across useful-rated findings.
	MeanUsefulConf float64 `json:"mean_useful_conf,omitempty"`

	// SuppressBelow is a confidence threshold below which findings for this rule
	// should be suppressed in future analyses. 0 means no suppression.
	// Populated by Recalculate when NoiseRate exceeds the suppression threshold.
	SuppressBelow float64 `json:"suppress_below,omitempty"`

	// LastUpdated is when this profile was last recalculated.
	LastUpdated time.Time `json:"last_updated"`
}

// Recalculate refreshes derived fields (NoiseRate, ConfCalibration, SuppressBelow)
// from the raw count and confidence fields. It must be called after any mutation
// to count or confidence fields to keep the profile consistent.
func (p *RuleProfile) Recalculate() {
	if p.TotalFindings > 0 {
		p.NoiseRate = float64(p.NoiseCount) / float64(p.TotalFindings)
	} else {
		p.NoiseRate = 0
	}

	// ConfCalibration requires both confidence measurements to be non-zero.
	if p.MeanUsefulConf != 0 || p.MeanNoiseConf != 0 {
		p.ConfCalibration = p.MeanUsefulConf - p.MeanNoiseConf
	} else {
		p.ConfCalibration = 0
	}

	// Set a suppression threshold when noise is high (>= 70% of findings are noise)
	// and we have confidence signal to act on.
	const suppressThreshold = 0.7
	if p.NoiseRate >= suppressThreshold && p.MeanNoiseConf > 0 {
		p.SuppressBelow = p.MeanNoiseConf
	}
}

// ThresholdOverride carries per-rule confidence thresholds that the calibration
// server pushes to the CLI so it can suppress low-quality findings locally.
type ThresholdOverride struct {
	// SuppressBelow is the confidence threshold below which findings for the
	// associated rule should be omitted from the analysis output.
	SuppressBelow float64 `json:"suppress_below"`
}

// CrossOrgSignal aggregates anonymised calibration signals across all teams for
// a rule. It is used to seed new-team priors and surface global noise patterns.
type CrossOrgSignal struct {
	// RuleID is the rule these stats describe.
	RuleID string `json:"rule_id"`

	// GlobalNoiseRate is the noise rate averaged across all teams that have
	// feedback for this rule.
	GlobalNoiseRate float64 `json:"global_noise_rate"`

	// TeamCount is the number of distinct teams contributing to this signal.
	TeamCount int `json:"team_count"`

	// TotalFeedbackEvents is the sum of all feedback events across teams.
	TotalFeedbackEvents int `json:"total_feedback_events"`

	// ComputedAt is when this cross-org signal was last computed.
	ComputedAt time.Time `json:"computed_at"`
}

// CalibrationResponse is the payload returned by the calibration server to the
// CLI after it uploads a batch of events. It carries updated thresholds and
// cross-org signals so the CLI can adjust future analysis output immediately.
type CalibrationResponse struct {
	// TeamThresholds maps rule ID to the updated threshold for that team.
	TeamThresholds map[string]ThresholdOverride `json:"team_thresholds,omitempty"`

	// CrossOrgSignals maps rule ID to the latest anonymised cross-org signal.
	CrossOrgSignals map[string]CrossOrgSignal `json:"cross_org_signals,omitempty"`
}
