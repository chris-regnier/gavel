package sarif

import (
	"regexp"
	"testing"
)

// makeResult returns a Result with a snippet populated, suitable for
// fingerprinting via SetContentFingerprint.
func makeResult(ruleID, uri, snippet string, startLine int) Result {
	return Result{
		RuleID:  ruleID,
		Level:   "warning",
		Message: Message{Text: "test"},
		Locations: []Location{{PhysicalLocation: PhysicalLocation{
			ArtifactLocation: ArtifactLocation{URI: uri},
			Region: Region{
				StartLine: startLine,
				EndLine:   startLine,
				Snippet:   &ArtifactContent{Text: snippet},
			},
		}}},
	}
}

// makeLog wraps a list of results into a Log, running SetContentFingerprint
// on each so they can participate in baseline comparison.
func makeLog(results ...Result) *Log {
	for i := range results {
		SetContentFingerprint(&results[i])
	}
	return &Log{
		Runs: []Run{{Results: results}},
	}
}

func TestCompareBaseline_MixedStates(t *testing.T) {
	baseline := makeLog(
		makeResult("SEC001", "a.go", "password := \"hunter2\"\n", 10),
		makeResult("SEC002", "b.go", "eval(userInput)\n", 20), // fixed in current
	)
	// Current keeps SEC001 (moved to a new line) and introduces SEC003.
	current := makeLog(
		makeResult("SEC001", "a.go", "password := \"hunter2\"\n", 42),
		makeResult("SEC003", "c.go", "os.Remove(userPath)\n", 5),
	)

	CompareBaseline(current, baseline)

	if got := len(current.Runs[0].Results); got != 3 {
		t.Fatalf("expected 3 results after baseline (2 current + 1 absent), got %d", got)
	}

	states := map[string]string{}
	for _, r := range current.Runs[0].Results {
		states[r.RuleID] = r.BaselineState
	}
	if states["SEC001"] != BaselineStateUnchanged {
		t.Errorf("SEC001 baselineState = %q, want unchanged (content fingerprint should survive line shift)", states["SEC001"])
	}
	if states["SEC003"] != BaselineStateNew {
		t.Errorf("SEC003 baselineState = %q, want new", states["SEC003"])
	}
	if states["SEC002"] != BaselineStateAbsent {
		t.Errorf("SEC002 baselineState = %q, want absent", states["SEC002"])
	}
}

func TestCompareBaseline_AllNew(t *testing.T) {
	baseline := makeLog() // no results
	current := makeLog(
		makeResult("SEC001", "a.go", "x := 1\n", 1),
		makeResult("SEC002", "b.go", "y := 2\n", 1),
	)

	CompareBaseline(current, baseline)

	for _, r := range current.Runs[0].Results {
		if r.BaselineState != BaselineStateNew {
			t.Errorf("%s baselineState = %q, want new", r.RuleID, r.BaselineState)
		}
	}
}

func TestCompareBaseline_AllUnchanged(t *testing.T) {
	shared := []Result{
		makeResult("SEC001", "a.go", "x := 1\n", 1),
		makeResult("SEC002", "b.go", "y := 2\n", 1),
	}
	baseline := makeLog(shared[0], shared[1])
	current := makeLog(shared[0], shared[1])

	CompareBaseline(current, baseline)

	if got := len(current.Runs[0].Results); got != 2 {
		t.Fatalf("expected 2 results, got %d", got)
	}
	for _, r := range current.Runs[0].Results {
		if r.BaselineState != BaselineStateUnchanged {
			t.Errorf("%s baselineState = %q, want unchanged", r.RuleID, r.BaselineState)
		}
	}
}

func TestCompareBaseline_CopiesBaselineGuid(t *testing.T) {
	baseline := makeLog(makeResult("SEC001", "a.go", "x\n", 1))
	baseline.Runs[0].AutomationDetails = &RunAutomationDetails{
		Guid: "baseline-guid-42",
	}
	current := makeLog(makeResult("SEC001", "a.go", "x\n", 1))

	CompareBaseline(current, baseline)

	if got := current.Runs[0].BaselineGuid; got != "baseline-guid-42" {
		t.Errorf("BaselineGuid = %q, want %q", got, "baseline-guid-42")
	}
}

func TestCompareBaseline_SkipsResultsWithoutFingerprint(t *testing.T) {
	// A result without a snippet gets no content fingerprint, so we can't
	// place it on either side of the comparison — it should be left alone.
	noSnippet := Result{
		RuleID:  "SEC001",
		Message: Message{Text: "no snippet"},
		Locations: []Location{{PhysicalLocation: PhysicalLocation{
			ArtifactLocation: ArtifactLocation{URI: "a.go"},
			Region:           Region{StartLine: 1, EndLine: 1},
		}}},
	}
	current := &Log{Runs: []Run{{Results: []Result{noSnippet}}}}
	baseline := makeLog(makeResult("SEC001", "a.go", "x\n", 1))

	CompareBaseline(current, baseline)

	if got := current.Runs[0].Results[0].BaselineState; got != "" {
		t.Errorf("expected empty baselineState for fingerprintless result, got %q", got)
	}
	// The baseline result still went absent (not seen in current).
	if got := len(current.Runs[0].Results); got != 2 {
		t.Errorf("expected 2 results (1 skipped + 1 absent), got %d", got)
	}
}

func TestCompareBaseline_NilLogsAreNoop(t *testing.T) {
	current := makeLog(makeResult("SEC001", "a.go", "x\n", 1))
	CompareBaseline(current, nil)
	if r := current.Runs[0].Results[0]; r.BaselineState != "" {
		t.Errorf("expected no mutation when baseline is nil, got baselineState=%q", r.BaselineState)
	}

	// Empty runs: also a no-op.
	CompareBaseline(&Log{}, &Log{})
}

func TestCompareBaseline_DoesNotMutateBaseline(t *testing.T) {
	baseline := makeLog(makeResult("SEC002", "b.go", "fixed\n", 1))
	current := makeLog(makeResult("SEC001", "a.go", "new\n", 1))

	CompareBaseline(current, baseline)

	// The baseline log must not be mutated — consumers may reuse it.
	if got := baseline.Runs[0].Results[0].BaselineState; got != "" {
		t.Errorf("baseline result should be untouched, got baselineState=%q", got)
	}
	if got := baseline.Runs[0].BaselineGuid; got != "" {
		t.Errorf("baseline run should be untouched, got baselineGuid=%q", got)
	}
}

func TestEnsureAutomationDetails_AssignsGUID(t *testing.T) {
	log := makeLog()
	EnsureAutomationDetails(log)

	if log.Runs[0].AutomationDetails == nil {
		t.Fatal("expected AutomationDetails to be set")
	}
	guid := log.Runs[0].AutomationDetails.Guid
	if guid == "" {
		t.Fatal("expected guid to be assigned")
	}
	// Looks like a v4 UUID: 8-4-4-4-12 lowercase hex with the version
	// nibble set to 4 and the RFC 4122 variant bits set.
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidRe.MatchString(guid) {
		t.Errorf("guid %q does not look like a v4 UUID", guid)
	}
}

func TestEnsureAutomationDetails_PreservesExisting(t *testing.T) {
	log := makeLog()
	log.Runs[0].AutomationDetails = &RunAutomationDetails{Guid: "caller-assigned"}
	EnsureAutomationDetails(log)

	if got := log.Runs[0].AutomationDetails.Guid; got != "caller-assigned" {
		t.Errorf("guid = %q, want caller-assigned to be preserved", got)
	}
}

func TestEnsureAutomationDetails_UniquePerCall(t *testing.T) {
	a := makeLog()
	b := makeLog()
	EnsureAutomationDetails(a)
	EnsureAutomationDetails(b)
	if a.Runs[0].AutomationDetails.Guid == b.Runs[0].AutomationDetails.Guid {
		t.Error("expected distinct guids for separate calls")
	}
}
