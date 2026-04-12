package sarif

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// ContentFingerprintV1 is the key under which Gavel stores its
// content-based SARIF fingerprint. Unlike the positional partialFingerprint
// "primaryLocationLineHash", this fingerprint depends only on the rule ID
// and the snippet text (with whitespace normalized), so it remains stable
// across line shifts and whitespace-only reformatting.
const ContentFingerprintV1 = "gavel/contentHash/v1"

// SetContentFingerprint computes the content-based fingerprint for a result
// and writes it into r.Fingerprints[ContentFingerprintV1]. If the result has
// no snippet (or the snippet contains no non-whitespace content), no
// fingerprint is set and r is left unchanged. Calling this function on a
// result that already has a content fingerprint overwrites the existing
// value; it is idempotent when inputs are unchanged.
func SetContentFingerprint(r *Result) {
	if r == nil || len(r.Locations) == 0 {
		return
	}
	snippet := r.Locations[0].PhysicalLocation.Region.Snippet
	if snippet == nil {
		return
	}
	normalized := normalizeSnippet(snippet.Text)
	if normalized == "" {
		return
	}
	if r.Fingerprints == nil {
		r.Fingerprints = make(map[string]string)
	}
	input := r.RuleID + "\n" + normalized
	hash := sha256.Sum256([]byte(input))
	r.Fingerprints[ContentFingerprintV1] = fmt.Sprintf("%x", hash[:16])
}

// normalizeSnippet returns a whitespace-normalized form of a code snippet
// suitable for content-based fingerprinting. Each line is trimmed of leading
// and trailing whitespace and blank lines are dropped. Returns "" if the
// snippet contains no non-whitespace content.
func normalizeSnippet(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}
