package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/chris-regnier/gavel/internal/sarif"
)

var storeTracer = otel.Tracer("github.com/chris-regnier/gavel/internal/store")

type FileStore struct {
	dir string
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

func (s *FileStore) generateID() string {
	b := make([]byte, 3)
	rand.Read(b)
	ts := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("%s-%s", ts, hex.EncodeToString(b))
}

func (s *FileStore) resultDir(id string) string {
	return filepath.Join(s.dir, id)
}

func (s *FileStore) WriteSARIF(ctx context.Context, doc *sarif.Log) (string, error) {
	ctx, span := storeTracer.Start(ctx, "write sarif")
	defer span.End()

	id := s.generateID()
	dir := s.resultDir(id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "sarif.json"), data, 0644); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	resultCount := 0
	if len(doc.Runs) > 0 {
		resultCount = len(doc.Runs[0].Results)
	}
	span.SetAttributes(
		attribute.String("gavel.store.id", id),
		attribute.Int("gavel.store.result_count", resultCount),
	)
	return id, nil
}

func (s *FileStore) WriteVerdict(ctx context.Context, sarifID string, verdict *Verdict) error {
	_, span := storeTracer.Start(ctx, "write verdict")
	defer span.End()

	dir := s.resultDir(sarifID)
	data, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "verdict.json"), data, 0644); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(
		attribute.String("gavel.store.id", sarifID),
		attribute.String("gavel.decision", verdict.Decision),
	)
	return nil
}

func (s *FileStore) ReadSARIF(ctx context.Context, id string) (*sarif.Log, error) {
	data, err := os.ReadFile(filepath.Join(s.resultDir(id), "sarif.json"))
	if err != nil {
		return nil, err
	}
	var log sarif.Log
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	return &log, nil
}

func (s *FileStore) ReadVerdict(ctx context.Context, sarifID string) (*Verdict, error) {
	data, err := os.ReadFile(filepath.Join(s.resultDir(sarifID), "verdict.json"))
	if err != nil {
		return nil, err
	}
	var v Verdict
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *FileStore) List(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids, nil
}
