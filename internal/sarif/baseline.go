package sarif

import (
	"crypto/rand"
	"fmt"
)

// BaselineState values per SARIF 2.1.0 §3.27.19.
const (
	BaselineStateNew       = "new"
	BaselineStateUnchanged = "unchanged"
	BaselineStateUpdated   = "updated"
	BaselineStateAbsent    = "absent"
)

// CompareBaseline annotates each result in current with a baselineState
// relative to baseline and links current to baseline via baselineGuid.
//
// Matching uses the content-based fingerprint (ContentFingerprintV1):
//
//   - unchanged: a current result whose fingerprint exists in baseline
//   - new:       a current result whose fingerprint is absent from baseline
//   - absent:    a baseline result whose fingerprint is absent from current,
//     appended to current.Runs[0].Results so downstream consumers
//     (dashboards, gating rules) can see what was fixed
//
// Results in current that lack a content fingerprint are left untouched:
// without a stable identifier we cannot place them on either side of the
// comparison. CompareBaseline is a no-op if either log is nil or has no
// runs.
func CompareBaseline(current, baseline *Log) {
	if current == nil || baseline == nil {
		return
	}
	if len(current.Runs) == 0 || len(baseline.Runs) == 0 {
		return
	}
	curRun := &current.Runs[0]
	baseRun := &baseline.Runs[0]

	// Link to the baseline run's automation guid, if present, so downstream
	// consumers can walk the chain of runs.
	if baseRun.AutomationDetails != nil && baseRun.AutomationDetails.Guid != "" {
		curRun.BaselineGuid = baseRun.AutomationDetails.Guid
	}

	// Index the baseline by content fingerprint -> the baseline result, so
	// we can both detect matches and surface any that went absent.
	baselineByFingerprint := make(map[string]int, len(baseRun.Results))
	for i, r := range baseRun.Results {
		fp := contentFingerprint(r)
		if fp == "" {
			continue
		}
		// Keep the first occurrence; dedup should make this irrelevant in
		// practice and the remaining duplicates still get indexed for the
		// "seen" check below.
		if _, exists := baselineByFingerprint[fp]; !exists {
			baselineByFingerprint[fp] = i
		}
	}

	// Walk current results: unchanged if fingerprint appears in baseline,
	// new otherwise. Fingerprintless results are skipped.
	seen := make(map[string]bool, len(curRun.Results))
	for i := range curRun.Results {
		r := &curRun.Results[i]
		fp := contentFingerprint(*r)
		if fp == "" {
			continue
		}
		if _, ok := baselineByFingerprint[fp]; ok {
			r.BaselineState = BaselineStateUnchanged
		} else {
			r.BaselineState = BaselineStateNew
		}
		seen[fp] = true
	}

	// Append absent findings: anything in baseline whose fingerprint did
	// not reappear in current. We copy the baseline result so mutations on
	// current don't leak back into the caller's baseline log.
	for _, r := range baseRun.Results {
		fp := contentFingerprint(r)
		if fp == "" || seen[fp] {
			continue
		}
		absent := r
		absent.BaselineState = BaselineStateAbsent
		curRun.Results = append(curRun.Results, absent)
	}
}

// EnsureAutomationDetails sets a fresh automation GUID on the run if it is
// missing. Call this before storing a new SARIF log so subsequent runs can
// reference it via BaselineGuid. Existing GUIDs are left alone so callers
// that assign their own identifier (e.g. CI run id) are respected.
func EnsureAutomationDetails(log *Log) {
	if log == nil || len(log.Runs) == 0 {
		return
	}
	run := &log.Runs[0]
	if run.AutomationDetails == nil {
		run.AutomationDetails = &RunAutomationDetails{}
	}
	if run.AutomationDetails.Guid == "" {
		run.AutomationDetails.Guid = newGUID()
	}
}

// contentFingerprint returns the content-based fingerprint for r, or "" if
// none is set.
func contentFingerprint(r Result) string {
	if r.Fingerprints == nil {
		return ""
	}
	return r.Fingerprints[ContentFingerprintV1]
}

// newGUID returns a random RFC 4122 version 4 UUID string. Falls back to a
// plain hex string if the OS RNG fails (crypto/rand effectively never
// does, so the fallback is defensive).
func newGUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%032x", b[:])
	}
	// Set version (4) and variant (RFC 4122) bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
