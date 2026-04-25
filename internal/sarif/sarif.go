package sarif

const SchemaURI = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json"
const Version = "2.1.0"

type Log struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

type Run struct {
	Tool              Tool                   `json:"tool"`
	Results           []Result               `json:"results"`
	Taxonomies        []ToolComponent        `json:"taxonomies,omitempty"`
	Invocations       []Invocation           `json:"invocations,omitempty"`
	AutomationDetails *RunAutomationDetails  `json:"automationDetails,omitempty"`
	BaselineGuid      string                 `json:"baselineGuid,omitempty"`
	Properties        map[string]interface{} `json:"properties,omitempty"`
}

// RunAutomationDetails identifies a single analysis run, per SARIF 2.1.0
// §3.17. The Guid is the stable identifier subsequent runs use to link back
// to this one via Run.BaselineGuid.
type RunAutomationDetails struct {
	ID   string `json:"id,omitempty"`
	Guid string `json:"guid,omitempty"`
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
	ID            string                  `json:"id"`
	ToolComponent *ToolComponentReference `json:"toolComponent,omitempty"`
}

// ToolComponentReference identifies a toolComponent by name.
type ToolComponentReference struct {
	Name string `json:"name"`
}

// ToolComponent represents a SARIF toolComponent (§3.19). Used inside
// Run.Taxonomies to describe a taxonomy (e.g., CWE, OWASP) and its taxa.
type ToolComponent struct {
	Name         string  `json:"name"`
	Organization string  `json:"organization,omitempty"`
	Taxa         []Taxon `json:"taxa,omitempty"`
}

// Taxon represents an entry in a taxonomy (a reportingDescriptor used as
// a taxon, §3.19.6).
type Taxon struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
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
	RelatedLocations    []Location             `json:"relatedLocations,omitempty"`
	CodeFlows           []CodeFlow             `json:"codeFlows,omitempty"`
	Fingerprints        map[string]string      `json:"fingerprints,omitempty"`
	PartialFingerprints map[string]string      `json:"partialFingerprints,omitempty"`
	BaselineState       string                 `json:"baselineState,omitempty"`
	Properties          map[string]interface{} `json:"properties,omitempty"`
	Suppressions        []SARIFSuppression     `json:"suppressions,omitempty"`
	Fixes               []Fix                  `json:"fixes,omitempty"`
}

// CodeFlow represents a SARIF codeFlow (§3.36): an ordered sequence of
// threadFlows describing how an issue arises step-by-step (e.g. tainted
// input → propagation → sink). SARIF viewers like GitHub Code Scanning and
// VS Code render each step as a navigable trace.
type CodeFlow struct {
	Message     *Message     `json:"message,omitempty"`
	ThreadFlows []ThreadFlow `json:"threadFlows"`
}

// ThreadFlow represents a SARIF threadFlow (§3.37): an ordered list of
// locations executed within a single thread of analysis.
type ThreadFlow struct {
	Locations []ThreadFlowLocation `json:"locations"`
}

// ThreadFlowLocation represents a SARIF threadFlowLocation (§3.38): a single
// hop in a threadFlow, wrapping the location reached at this step.
type ThreadFlowLocation struct {
	Location *Location `json:"location,omitempty"`
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
	PhysicalLocation PhysicalLocation  `json:"physicalLocation"`
	LogicalLocations []LogicalLocation `json:"logicalLocations,omitempty"`
	Message          *Message          `json:"message,omitempty"`
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

// Fix represents a proposed fix for a result, per SARIF 2.1.0 §3.55.
// Downstream tools (LSP, GitHub Code Scanning, auto-fix bots) can apply
// the replacements structurally to remediate the finding.
type Fix struct {
	Description     Message          `json:"description,omitempty"`
	ArtifactChanges []ArtifactChange `json:"artifactChanges"`
}

// ArtifactChange represents a set of replacements applied to a single
// artifact (source file), per SARIF 2.1.0 §3.56.
type ArtifactChange struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Replacements     []Replacement    `json:"replacements"`
}

// Replacement represents a single deletion-plus-insertion within an artifact,
// per SARIF 2.1.0 §3.57. InsertedContent is optional — omitting it expresses
// a pure deletion.
type Replacement struct {
	DeletedRegion   Region           `json:"deletedRegion"`
	InsertedContent *ArtifactContent `json:"insertedContent,omitempty"`
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
