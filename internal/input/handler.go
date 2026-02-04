package input

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Kind int

const (
	KindFile Kind = iota
	KindDiff
)

type Artifact struct {
	Path    string
	Content string
	Kind    Kind
}

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ReadFiles(paths []string) ([]Artifact, error) {
	var artifacts []Artifact
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(data) {
			slog.Warn("skipping file with invalid UTF-8", "path", p)
			continue
		}
		artifacts = append(artifacts, Artifact{
			Path:    p,
			Content: string(data),
			Kind:    KindFile,
		})
	}
	return artifacts, nil
}

func (h *Handler) ReadDiff(diff string) ([]Artifact, error) {
	var artifacts []Artifact
	var currentPath string
	var currentLines []string

	flush := func() {
		if currentPath != "" {
			artifacts = append(artifacts, Artifact{
				Path:    currentPath,
				Content: strings.Join(currentLines, "\n"),
				Kind:    KindDiff,
			})
		}
	}

	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			flush()
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				currentPath = strings.TrimPrefix(parts[len(parts)-1], "b/")
			}
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	flush()

	return artifacts, nil
}

func (h *Handler) ReadDirectory(dir string) ([]Artifact, error) {
	var artifacts []Artifact
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			slog.Warn("skipping file with invalid UTF-8", "path", path)
			return nil
		}
		artifacts = append(artifacts, Artifact{
			Path:    path,
			Content: string(data),
			Kind:    KindFile,
		})
		return nil
	})
	return artifacts, err
}
