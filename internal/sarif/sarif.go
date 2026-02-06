package sarif

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const SchemaURI = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json"
const Version = "2.1.0"

type Log struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

type Run struct {
	Tool       Tool                   `json:"tool"`
	Results    []Result               `json:"results"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type Tool struct {
	Driver Driver `json:"driver"`
}

type Driver struct {
	Name           string                `json:"name"`
	Version        string                `json:"version,omitempty"`
	InformationURI string                `json:"informationUri,omitempty"`
	Rules          []ReportingDescriptor `json:"rules,omitempty"`
}

type ReportingDescriptor struct {
	ID               string                  `json:"id"`
	ShortDescription Message                 `json:"shortDescription,omitempty"`
	DefaultConfig    *ReportingConfiguration `json:"defaultConfiguration,omitempty"`
}

type ReportingConfiguration struct {
	Level string `json:"level,omitempty"`
}

type Result struct {
	RuleID     string                 `json:"ruleId"`
	Level      string                 `json:"level"`
	Message    Message                `json:"message"`
	Locations  []Location             `json:"locations,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type Message struct {
	Text string `json:"text"`
}

type Location struct {
	PhysicalLocation PhysicalLocation `json:"physicalLocation"`
}

type PhysicalLocation struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Region           Region           `json:"region,omitempty"`
}

type ArtifactLocation struct {
	URI string `json:"uri"`
}

type Region struct {
	StartLine int `json:"startLine,omitempty"`
	EndLine   int `json:"endLine,omitempty"`
}

func NewLog(toolName, toolVersion string) *Log {
	return &Log{
		Schema:  SchemaURI,
		Version: Version,
		Runs: []Run{{
			Tool: Tool{
				Driver: Driver{
					Name:    toolName,
					Version: toolVersion,
				},
			},
			Results: []Result{},
		}},
	}
}

// CacheMetadata represents metadata for content-addressable caching
type CacheMetadata struct {
	FileHash    string
	Provider    string
	Model       string
	BAMLVersion string
	Policies    map[string]PolicyMetadata
}

// PolicyMetadata represents policy configuration for cache key
type PolicyMetadata struct {
	Instruction string
	Version     string
}

// ComputeCacheKey generates deterministic hash from cache metadata
func (m *CacheMetadata) ComputeCacheKey() string {
	data := struct {
		File     string
		Provider string
		Model    string
		BAML     string
		Policies map[string]PolicyMetadata
	}{
		File:     m.FileHash,
		Provider: m.Provider,
		Model:    m.Model,
		BAML:     m.BAMLVersion,
		Policies: m.Policies,
	}

	b, _ := json.Marshal(data)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
