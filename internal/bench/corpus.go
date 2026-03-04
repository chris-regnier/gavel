package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExpectedFinding defines a ground-truth finding for a corpus case.
type ExpectedFinding struct {
	RuleID    string `yaml:"rule_id"`    // Specific rule ID or "any"
	Severity  string `yaml:"severity"`   // "error", "warning", "note"
	LineRange [2]int `yaml:"line_range"` // [start, end] approximate
	Category  string `yaml:"category"`
	MustFind  bool   `yaml:"must_find"` // true = required for recall
}

// ExpectedManifest is the expected.yaml structure.
type ExpectedManifest struct {
	Findings       []ExpectedFinding `yaml:"findings"`
	FalsePositives int               `yaml:"false_positives"`
}

// CaseMetadata is the metadata.yaml structure.
type CaseMetadata struct {
	Name        string `yaml:"name"`
	Language    string `yaml:"language"`
	Category    string `yaml:"category"`
	Difficulty  string `yaml:"difficulty"`
	Description string `yaml:"description"`
}

// Case represents a single benchmark corpus test case.
type Case struct {
	Name             string
	Dir              string
	SourcePath       string
	SourceContent    string
	ExpectedFindings []ExpectedFinding
	FalsePositives   int
	Metadata         CaseMetadata
}

// Corpus is a collection of benchmark cases.
type Corpus struct {
	Dir   string
	Cases []Case
}

// LoadCase loads a single benchmark case from a directory.
func LoadCase(dir string) (*Case, error) {
	name := filepath.Base(dir)

	// Find source file (first file not named expected.yaml or metadata.yaml)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read case dir: %w", err)
	}

	var sourcePath string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if n == "expected.yaml" || n == "metadata.yaml" {
			continue
		}
		sourcePath = filepath.Join(dir, n)
		break
	}
	if sourcePath == "" {
		return nil, fmt.Errorf("no source file found in %s", dir)
	}

	sourceContent, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}

	// Load expected.yaml
	var manifest ExpectedManifest
	expectedData, err := os.ReadFile(filepath.Join(dir, "expected.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read expected.yaml: %w", err)
	}
	if err := yaml.Unmarshal(expectedData, &manifest); err != nil {
		return nil, fmt.Errorf("parse expected.yaml: %w", err)
	}

	// Load metadata.yaml (optional)
	var meta CaseMetadata
	metaData, err := os.ReadFile(filepath.Join(dir, "metadata.yaml"))
	if err == nil {
		yaml.Unmarshal(metaData, &meta)
	}

	return &Case{
		Name:             name,
		Dir:              dir,
		SourcePath:       sourcePath,
		SourceContent:    string(sourceContent),
		ExpectedFindings: manifest.Findings,
		FalsePositives:   manifest.FalsePositives,
		Metadata:         meta,
	}, nil
}

// LoadCorpus loads all cases from a corpus directory.
// Expected structure: corpus/<language>/<case-name>/
func LoadCorpus(dir string) (*Corpus, error) {
	corpus := &Corpus{Dir: dir}

	langDirs, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read corpus dir: %w", err)
	}

	for _, langDir := range langDirs {
		if !langDir.IsDir() || strings.HasPrefix(langDir.Name(), ".") {
			continue
		}
		langPath := filepath.Join(dir, langDir.Name())
		caseDirs, err := os.ReadDir(langPath)
		if err != nil {
			continue
		}
		for _, caseDir := range caseDirs {
			if !caseDir.IsDir() || strings.HasPrefix(caseDir.Name(), ".") {
				continue
			}
			c, err := LoadCase(filepath.Join(langPath, caseDir.Name()))
			if err != nil {
				return nil, fmt.Errorf("load case %s/%s: %w", langDir.Name(), caseDir.Name(), err)
			}
			corpus.Cases = append(corpus.Cases, *c)
		}
	}

	return corpus, nil
}
