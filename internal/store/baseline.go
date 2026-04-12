package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// LoadBaseline resolves ref to a baseline SARIF log. If ref points at an
// existing file it is decoded directly; otherwise ref is treated as a
// stored result ID and read via s.ReadSARIF. This lets callers accept
// either a downloaded CI artifact (`./prev/sarif.json`) or a local
// result ID (`2026-04-12T...-abcdef`) with a single interface.
func LoadBaseline(ctx context.Context, s Store, ref string) (*sarif.Log, error) {
	if info, err := os.Stat(ref); err == nil && !info.IsDir() {
		data, err := os.ReadFile(ref)
		if err != nil {
			return nil, err
		}
		var log sarif.Log
		if err := json.Unmarshal(data, &log); err != nil {
			return nil, fmt.Errorf("decoding baseline SARIF %q: %w", ref, err)
		}
		return &log, nil
	}
	return s.ReadSARIF(ctx, ref)
}
