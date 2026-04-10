package sarif

const SchemaURI = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json"
const Version = "2.1.0"

type Log struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

type Run struct {
	Tool        Tool                   `json:"tool"`
	Results     []Result               `json:"results"`
	Invocations []Invocation           `json:"invocations,omitempty"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
}

type Invocation struct {
	WorkingDirectory    ArtifactLocation `json:"workingDirectory"`
	ExecutionSuccessful bool             `json:"executionSuccessful"`
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
	Name             string                  `json:"name,omitempty"`
	ShortDescription Message                 `json:"shortDescription,omitempty"`
	FullDescription  *Message                `json:"fullDescription,omitempty"`
	Help             *MultiformatMessage     `json:"help,omitempty"`
	HelpURI          string                  `json:"helpUri,omitempty"`
	DefaultConfig    *ReportingConfiguration `json:"defaultConfiguration,omitempty"`
	Relationships    []Relationship          `json:"relationships,omitempty"`
}

// Relationship represents a reportingDescriptorRelationship (§3.52) on a
// rule descriptor, linking it to a taxon in an external taxonomy such as CWE
// or OWASP.
type Relationship struct {
	Target RelationshipTarget `json:"target"`
	Kinds  []string           `json:"kinds,omitempty"`
}

// RelationshipTarget identifies a specific taxon within a named toolComponent.
type RelationshipTarget struct {
	ID            string                 `json:"id"`
	ToolComponent *ToolComponentReference `json:"toolComponent,omitempty"`
}

// ToolComponentReference identifies a toolComponent by name.
type ToolComponentReference struct {
	Name string `json:"name"`
}

type ReportingConfiguration struct {
	Level string `json:"level,omitempty"`
}

// MultiformatMessage carries a message in both plain text and markdown forms,
// per SARIF 2.1.0 §3.11 (multiformatMessageString). SARIF viewers such as
// GitHub Code Scanning and VS Code render the markdown form as rule help.
type MultiformatMessage struct {
	Text     string `json:"text,omitempty"`
	Markdown string `json:"markdown,omitempty"`
}

type Result struct {
	RuleID              string                 `json:"ruleId"`
	Level               string                 `json:"level"`
	Message             Message                `json:"message"`
	Locations           []Location             `json:"locations,omitempty"`
	PartialFingerprints map[string]string      `json:"partialFingerprints,omitempty"`
	Properties          map[string]interface{} `json:"properties,omitempty"`
	Suppressions        []SARIFSuppression     `json:"suppressions,omitempty"`
}

type SARIFSuppression struct {
	Kind          string                 `json:"kind"`
	Justification string                 `json:"justification,omitempty"`
	Properties    map[string]interface{} `json:"properties,omitempty"`
}

type Message struct {
	Text string `json:"text"`
}

type Location struct {
	PhysicalLocation  PhysicalLocation  `json:"physicalLocation"`
	LogicalLocations []LogicalLocation `json:"logicalLocations,omitempty"`
}

// LogicalLocation provides semantic context (e.g. enclosing function/method/class)
// for a finding, per SARIF 2.1.0 §3.33.
type LogicalLocation struct {
	Name               string `json:"name,omitempty"`
	Kind               string `json:"kind,omitempty"`
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`
}

type PhysicalLocation struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Region           Region           `json:"region,omitempty"`
	ContextRegion    *Region          `json:"contextRegion,omitempty"`
}

type ArtifactLocation struct {
	URI string `json:"uri"`
}

type Region struct {
	StartLine int              `json:"startLine,omitempty"`
	EndLine   int              `json:"endLine,omitempty"`
	Snippet   *ArtifactContent `json:"snippet,omitempty"`
}

type ArtifactContent struct {
	Text string `json:"text"`
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
