package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// WorkItem represents a unit of work in the analysis pipeline
type WorkItem struct {
	ID        string
	FilePath  string
	Content   string
	Policies  string
	Persona   string
	Priority  int // Higher priority items are processed first
	CreatedAt time.Time
	CacheKey  string
}

// WorkResult represents the result of processing a work item
type WorkResult struct {
	ID        string
	CacheKey  string
	Result    interface{}
	Error     error
	FromCache bool
	Duration  time.Duration
}

// WorkFunc is the function signature for processing work items
type WorkFunc func(ctx context.Context, item WorkItem) (interface{}, error)

// Pipeline manages async analysis with caching
type Pipeline struct {
	cache     *Cache
	workFunc  WorkFunc
	workers   int
	
	// Channels
	workQueue   chan WorkItem
	resultChans sync.Map // map[string]chan WorkResult
	
	// State
	running   atomic.Bool
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	
	// Stats
	processed atomic.Int64
	errors    atomic.Int64
	cacheHits atomic.Int64
}

// PipelineOption configures a Pipeline
type PipelineOption func(*Pipeline)

// WithWorkers sets the number of worker goroutines
func WithWorkers(n int) PipelineOption {
	return func(p *Pipeline) {
		p.workers = n
	}
}

// WithCache sets the cache to use
func WithCache(c *Cache) PipelineOption {
	return func(p *Pipeline) {
		p.cache = c
	}
}

// WithQueueSize sets the work queue buffer size
func WithQueueSize(n int) PipelineOption {
	return func(p *Pipeline) {
		p.workQueue = make(chan WorkItem, n)
	}
}

// NewPipeline creates a new analysis pipeline
func NewPipeline(workFunc WorkFunc, opts ...PipelineOption) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())
	
	p := &Pipeline{
		cache:     New(),
		workFunc:  workFunc,
		workers:   2,
		workQueue: make(chan WorkItem, 100),
		ctx:       ctx,
		cancel:    cancel,
	}
	
	for _, opt := range opts {
		opt(p)
	}
	
	return p
}

// Start begins processing work items
func (p *Pipeline) Start() {
	if p.running.Swap(true) {
		return // Already running
	}
	
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Stop gracefully shuts down the pipeline
func (p *Pipeline) Stop() {
	if !p.running.Swap(false) {
		return // Already stopped
	}
	
	p.cancel()
	close(p.workQueue)
	p.wg.Wait()
}

// Submit adds a work item to the queue
// Returns a channel that will receive the result
func (p *Pipeline) Submit(item WorkItem) <-chan WorkResult {
	// Generate cache key if not set
	if item.CacheKey == "" {
		item.CacheKey = ContentKey(item.Content, item.Policies, item.Persona)
	}
	
	// Create result channel
	resultChan := make(chan WorkResult, 1)
	
	// Check cache first (before storing result channel)
	if cached, ok := p.cache.Get(item.CacheKey); ok {
		p.cacheHits.Add(1)
		result := WorkResult{
			ID:        item.ID,
			CacheKey:  item.CacheKey,
			Result:    cached,
			FromCache: true,
			Duration:  0,
		}
		resultChan <- result
		close(resultChan)
		return resultChan
	}
	
	// Store result channel only if we're going to queue the item
	p.resultChans.Store(item.ID, resultChan)
	
	// Submit to work queue
	select {
	case p.workQueue <- item:
		// Submitted successfully
	case <-p.ctx.Done():
		// Pipeline stopped - remove from map and send error
		p.resultChans.Delete(item.ID)
		result := WorkResult{
			ID:       item.ID,
			CacheKey: item.CacheKey,
			Error:    p.ctx.Err(),
		}
		resultChan <- result
		close(resultChan)
	}
	
	return resultChan
}

// SubmitAndWait submits a work item and waits for the result
func (p *Pipeline) SubmitAndWait(ctx context.Context, item WorkItem) WorkResult {
	resultChan := p.Submit(item)
	
	select {
	case result := <-resultChan:
		return result
	case <-ctx.Done():
		return WorkResult{
			ID:       item.ID,
			CacheKey: item.CacheKey,
			Error:    ctx.Err(),
		}
	}
}

// worker processes items from the work queue
func (p *Pipeline) worker(id int) {
	defer p.wg.Done()
	
	for item := range p.workQueue {
		p.processItem(item)
	}
}

// processItem processes a single work item
func (p *Pipeline) processItem(item WorkItem) {
	// Get result channel
	resultChanI, ok := p.resultChans.LoadAndDelete(item.ID)
	if !ok {
		return // No one waiting for this result
	}
	resultChan := resultChanI.(chan WorkResult)
	defer close(resultChan)
	
	// Check cache again (might have been populated by another worker)
	if cached, ok := p.cache.Get(item.CacheKey); ok {
		p.cacheHits.Add(1)
		resultChan <- WorkResult{
			ID:        item.ID,
			CacheKey:  item.CacheKey,
			Result:    cached,
			FromCache: true,
			Duration:  0,
		}
		return
	}
	
	// Process the item
	start := time.Now()
	result, err := p.workFunc(p.ctx, item)
	duration := time.Since(start)
	
	p.processed.Add(1)
	if err != nil {
		p.errors.Add(1)
		resultChan <- WorkResult{
			ID:       item.ID,
			CacheKey: item.CacheKey,
			Error:    err,
			Duration: duration,
		}
		return
	}
	
	// Cache the result
	p.cache.Set(item.CacheKey, result)
	
	resultChan <- WorkResult{
		ID:        item.ID,
		CacheKey:  item.CacheKey,
		Result:    result,
		FromCache: false,
		Duration:  duration,
	}
}

// Stats returns pipeline statistics
type PipelineStats struct {
	Processed  int64      `json:"processed"`
	Errors     int64      `json:"errors"`
	CacheHits  int64      `json:"cache_hits"`
	QueueDepth int        `json:"queue_depth"`
	CacheStats CacheStats `json:"cache_stats"`
}

// Stats returns current pipeline statistics
func (p *Pipeline) Stats() PipelineStats {
	return PipelineStats{
		Processed:  p.processed.Load(),
		Errors:     p.errors.Load(),
		CacheHits:  p.cacheHits.Load(),
		QueueDepth: len(p.workQueue),
		CacheStats: p.cache.Stats(),
	}
}

// GetCache returns the underlying cache
func (p *Pipeline) GetCache() *Cache {
	return p.cache
}
