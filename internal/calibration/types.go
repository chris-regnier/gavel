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
