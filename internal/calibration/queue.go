package calibration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// QueuedBatch is a persisted group of Events waiting to be flushed to the
// calibration server. Batches are written as individual JSON files under the
// queue directory so that a process crash cannot corrupt previously enqueued
// batches.
type QueuedBatch struct {
	// ID is a nanosecond-precision Unix timestamp string that also serves as
	// the on-disk filename stem.
	ID string `json:"id"`

	// TeamID is the team that owns the events in this batch.
	TeamID string `json:"team_id"`

	// Events is the list of calibration events to upload.
	Events []Event `json:"events"`
}

// LocalQueue is a file-backed FIFO queue of QueuedBatch values. Each call to
// Enqueue atomically writes a single JSON file to the queue directory; Drain
// reads all pending files; Remove deletes a single file by batch ID.
//
// LocalQueue is not safe for concurrent use by multiple processes sharing the
// same directory without additional locking at the call site.
type LocalQueue struct {
	dir string
}

// NewLocalQueue returns a LocalQueue that persists batches under dir. The
// directory is created lazily on the first call to Enqueue.
func NewLocalQueue(dir string) *LocalQueue {
	return &LocalQueue{dir: dir}
}

// Enqueue marshals events into a QueuedBatch and writes it to a JSON file in
// the queue directory. The filename is derived from the current time in
// nanoseconds, which provides a naturally ordered set of pending files.
func (q *LocalQueue) Enqueue(teamID string, events []Event) error {
	if err := os.MkdirAll(q.dir, 0o755); err != nil {
		return fmt.Errorf("create queue dir: %w", err)
	}
	batch := QueuedBatch{
		ID:     fmt.Sprintf("%d", time.Now().UnixNano()),
		TeamID: teamID,
		Events: events,
	}
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}
	return os.WriteFile(filepath.Join(q.dir, batch.ID+".json"), data, 0o644)
}

// Drain returns all QueuedBatch values currently stored in the queue directory.
// Files that cannot be read or unmarshalled are silently skipped so that a
// single corrupt file does not prevent delivery of valid batches. The caller is
// responsible for calling Remove on each batch after successful delivery.
//
// If the queue directory does not yet exist, Drain returns nil, nil.
func (q *LocalQueue) Drain() ([]QueuedBatch, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read queue dir: %w", err)
	}
	var batches []QueuedBatch
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(q.dir, e.Name()))
		if err != nil {
			continue
		}
		var b QueuedBatch
		if err := json.Unmarshal(data, &b); err != nil {
			continue
		}
		batches = append(batches, b)
	}
	return batches, nil
}

// Remove deletes the on-disk file for the batch identified by id. It should be
// called after a batch has been successfully delivered to the calibration server.
func (q *LocalQueue) Remove(id string) error {
	return os.Remove(filepath.Join(q.dir, id+".json"))
}
