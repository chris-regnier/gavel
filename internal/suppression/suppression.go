package suppression

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chris-regnier/gavel/internal/sarif"
	"gopkg.in/yaml.v3"
)

// Suppression represents a single suppression entry that silences a finding.
type Suppression struct {
	RuleID  string    `yaml:"rule_id"`
	File    string    `yaml:"file,omitempty"`
	Reason  string    `yaml:"reason"`
	Created time.Time `yaml:"created"`
	Source  string    `yaml:"source"`
}

// suppressionFile is the on-disk YAML structure wrapping the list.
type suppressionFile struct {
	Suppressions []Suppression `yaml:"suppressions"`
}

// suppressionsPath returns the canonical path to the suppressions file.
func suppressionsPath(projectDir string) string {
	return filepath.Join(projectDir, ".gavel", "suppressions.yaml")
}

// Load reads suppressions from projectDir/.gavel/suppressions.yaml.
// Returns an empty list (not an error) if the file does not exist.
func Load(projectDir string) ([]Suppression, error) {
	path := suppressionsPath(projectDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Suppression{}, nil
		}
		return nil, err
	}

	var sf suppressionFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	if sf.Suppressions == nil {
		return []Suppression{}, nil
	}
	return sf.Suppressions, nil
}

// Save writes suppressions to projectDir/.gavel/suppressions.yaml.
// Creates the .gavel/ directory if it does not exist.
func Save(projectDir string, suppressions []Suppression) error {
	dir := filepath.Join(projectDir, ".gavel")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	sf := suppressionFile{Suppressions: suppressions}
	data, err := yaml.Marshal(sf)
	if err != nil {
		return err
	}

	return os.WriteFile(suppressionsPath(projectDir), data, 0o644)
}

// NormalizePath converts a path to canonical form: forward slashes, no leading
// "./" prefix, cleaned via filepath.Clean.
func NormalizePath(p string) string {
	// Replace backslashes with forward slashes first so filepath.Clean
	// operates on a unified separator on all platforms.
	p = strings.ReplaceAll(p, `\`, "/")
	p = filepath.ToSlash(filepath.Clean(p))
	p = strings.TrimPrefix(p, "./")
	return p
}

// Apply clears existing suppression annotations on all results, then stamps
// matching results with SARIF-native suppression entries. This clear-then-apply
// approach ensures removed suppressions take effect correctly.
func Apply(suppressions []Suppression, log *sarif.Log) {
	for i := range log.Runs {
		for j := range log.Runs[i].Results {
			r := &log.Runs[i].Results[j]
			r.Suppressions = nil

			filePath := ""
			if len(r.Locations) > 0 {
				filePath = r.Locations[0].PhysicalLocation.ArtifactLocation.URI
			}

			s := Match(suppressions, r.RuleID, filePath)
			if s == nil {
				continue
			}

			r.Suppressions = []sarif.SARIFSuppression{
				{
					Kind:          "external",
					Justification: s.Reason,
					Properties: map[string]interface{}{
						"gavel/source":  s.Source,
						"gavel/created": s.Created.Format(time.RFC3339),
					},
				},
			}
		}
	}
}

// Match returns the first suppression that matches the given ruleID and filePath,
// or nil if none match. Global suppressions (empty File field) match any file.
// Per-file suppressions match only when normalized paths are equal.
func Match(suppressions []Suppression, ruleID string, filePath string) *Suppression {
	normalizedFile := NormalizePath(filePath)
	for i := range suppressions {
		s := &suppressions[i]
		if s.RuleID != ruleID {
			continue
		}
		if s.File == "" {
			return s
		}
		if NormalizePath(s.File) == normalizedFile {
			return s
		}
	}
	return nil
}
