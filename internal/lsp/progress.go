// internal/lsp/progress.go
package lsp

import (
	"context"
	"sync"
)

// ProgressReporter handles work done progress reporting
type ProgressReporter struct {
	send func(msg jsonRPCMessage) error
	mu   sync.Mutex
}

// NewProgressReporter creates a new progress reporter
func NewProgressReporter(send func(msg jsonRPCMessage) error) *ProgressReporter {
	return &ProgressReporter{send: send}
}

// Begin starts a new progress report
func (p *ProgressReporter) Begin(ctx context.Context, token string, title string, total int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Create the progress token first
	createParams := WorkDoneProgressCreateParams{
		Token: token,
	}
	createMsg := jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      "progress-create-" + token,
		Method:  MethodWindowWorkDoneProgressCreate,
		Params:  mustMarshal(createParams),
	}
	if err := p.send(createMsg); err != nil {
		return err
	}

	// Send begin notification
	beginValue := WorkDoneProgressBegin{
		Kind:        "begin",
		Title:       title,
		Cancellable: false,
		Percentage:  0,
	}
	progressParams := ProgressParams{
		Token: token,
		Value: beginValue,
	}
	notification := jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  MethodProgress,
		Params:  mustMarshal(progressParams),
	}
	return p.send(notification)
}

// Report sends an intermediate progress report
func (p *ProgressReporter) Report(ctx context.Context, token string, message string, current int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	reportValue := WorkDoneProgressReport{
		Kind:    "report",
		Message: message,
	}
	progressParams := ProgressParams{
		Token: token,
		Value: reportValue,
	}
	notification := jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  MethodProgress,
		Params:  mustMarshal(progressParams),
	}
	return p.send(notification)
}

// ReportWithPercentage sends an intermediate progress report with percentage
func (p *ProgressReporter) ReportWithPercentage(ctx context.Context, token string, message string, percentage int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	reportValue := WorkDoneProgressReport{
		Kind:       "report",
		Message:    message,
		Percentage: percentage,
	}
	progressParams := ProgressParams{
		Token: token,
		Value: reportValue,
	}
	notification := jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  MethodProgress,
		Params:  mustMarshal(progressParams),
	}
	return p.send(notification)
}

// End completes a progress report
func (p *ProgressReporter) End(ctx context.Context, token string, message string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	endValue := WorkDoneProgressEnd{
		Kind:    "end",
		Message: message,
	}
	progressParams := ProgressParams{
		Token: token,
		Value: endValue,
	}
	notification := jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  MethodProgress,
		Params:  mustMarshal(progressParams),
	}
	return p.send(notification)
}
