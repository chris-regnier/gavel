package feedback

import "testing"

func TestMapAlertStateToVerdict(t *testing.T) {
	tests := []struct {
		state   string
		reason  string
		want    Verdict
	}{
		{"fixed", "", VerdictUseful},
		{"dismissed", "false positive", VerdictWrong},
		{"dismissed", "won't fix", VerdictNoise},
		{"dismissed", "used in tests", VerdictNoise},
		{"dismissed", "", VerdictNoise},
		{"open", "", ""},
	}

	for _, tt := range tests {
		got := mapAlertStateToVerdict(tt.state, tt.reason)
		if got != tt.want {
			t.Errorf("mapAlertStateToVerdict(%q, %q) = %q, want %q", tt.state, tt.reason, got, tt.want)
		}
	}
}

func TestFormatGitHubReason(t *testing.T) {
	alert := GitHubAlert{
		Number:          42,
		State:           "dismissed",
		DismissedReason: "false positive",
	}
	alert.DismissedBy.Login = "dev1"

	reason := formatGitHubReason(alert)
	if reason == "" {
		t.Error("reason should not be empty")
	}
	if reason != "github alert #42, dismissed: false positive, by dev1" {
		t.Errorf("unexpected reason: %s", reason)
	}
}
