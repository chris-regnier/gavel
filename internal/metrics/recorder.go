package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Recorder provides a convenient API for recording analysis metrics
type Recorder struct {
	collector *Collector
	provider  string
	model     string
}

// NewRecorder creates a new metrics recorder
func NewRecorder(collector *Collector, provider, model string) *Recorder {
	return &Recorder{
		collector: collector,
		provider:  provider,
		model:     model,
	}
}

// AnalysisBuilder helps build an AnalysisEvent incrementally
type AnalysisBuilder struct {
	recorder *Recorder
	event    AnalysisEvent
	timing   *AnalysisTiming
	mu       sync.Mutex
}

// StartAnalysis begins recording a new analysis operation
func (r *Recorder) StartAnalysis(analysisType AnalysisType, tier TierLevel) *AnalysisBuilder {
	return &AnalysisBuilder{
		recorder: r,
		event: AnalysisEvent{
			ID:        generateID(),
			Timestamp: time.Now(),
			Type:      analysisType,
			Tier:      tier,
			Provider:  r.provider,
			Model:     r.model,
		},
		timing: NewTiming(),
	}
}

// WithFile sets the file information for the analysis
func (b *AnalysisBuilder) WithFile(path string, content string) *AnalysisBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.event.FilePath = path
	b.event.FileSize = len(content)
	b.event.LineCount = strings.Count(content, "\n") + 1
	return b
}

// WithChunks sets the chunk count for chunked analysis
func (b *AnalysisBuilder) WithChunks(count int) *AnalysisBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.event.ChunkCount = count
	return b
}

// WithPolicies sets the policy count
func (b *AnalysisBuilder) WithPolicies(count int) *AnalysisBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.event.PolicyCount = count
	return b
}

// WithCacheResult records a cache lookup result
func (b *AnalysisBuilder) WithCacheResult(result CacheResult, key string) *AnalysisBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.event.CacheResult = result
	b.event.CacheKey = key
	return b
}

// MarkStarted marks the analysis as started (dequeued)
func (b *AnalysisBuilder) MarkStarted() *AnalysisBuilder {
	b.timing.Start()
	return b
}

// Complete finishes recording and submits the event
func (b *AnalysisBuilder) Complete(findingCount int, tokensIn, tokensOut int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.timing.Complete()

	b.event.FindingCount = findingCount
	b.event.TokensIn = tokensIn
	b.event.TokensOut = tokensOut
	b.event.QueueDuration = b.timing.QueueDuration()
	b.event.AnalysisDuration = b.timing.AnalysisDuration()
	b.event.TotalDuration = b.timing.TotalDuration()

	b.recorder.collector.Record(b.event)
}

// CompleteWithError finishes recording with an error
func (b *AnalysisBuilder) CompleteWithError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.timing.Complete()

	b.event.Error = err.Error()
	b.event.ErrorCount = 1
	b.event.QueueDuration = b.timing.QueueDuration()
	b.event.AnalysisDuration = b.timing.AnalysisDuration()
	b.event.TotalDuration = b.timing.TotalDuration()

	b.recorder.collector.Record(b.event)
}

// generateID generates a unique ID for an analysis event
func generateID() string {
	now := time.Now()
	data := fmt.Sprintf("%d-%d", now.UnixNano(), now.Nanosecond())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// GenerateCacheKey creates a cache key from content and policies
func GenerateCacheKey(content, policies, persona string) string {
	h := sha256.New()
	h.Write([]byte(content))
	h.Write([]byte{0})
	h.Write([]byte(policies))
	h.Write([]byte{0})
	h.Write([]byte(persona))
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// ContextKey is the type for context keys used by the metrics package
type contextKey string

const recorderContextKey contextKey = "metrics_recorder"

// WithRecorder adds a recorder to the context
func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	return context.WithValue(ctx, recorderContextKey, recorder)
}

// RecorderFromContext retrieves a recorder from the context
func RecorderFromContext(ctx context.Context) *Recorder {
	if r, ok := ctx.Value(recorderContextKey).(*Recorder); ok {
		return r
	}
	return nil
}

// NoOpRecorder returns a recorder that discards all metrics
func NoOpRecorder() *Recorder {
	return &Recorder{
		collector: NewCollector(WithMaxEvents(0)),
	}
}
