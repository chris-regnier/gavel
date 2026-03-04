package feedback

import "testing"

func TestAggregateStats(t *testing.T) {
	entries := []Entry{
		{Verdict: VerdictUseful},
		{Verdict: VerdictUseful},
		{Verdict: VerdictNoise},
		{Verdict: VerdictWrong},
	}
	stats := ComputeStats(entries)
	if stats.Total != 4 {
		t.Errorf("Total = %d, want 4", stats.Total)
	}
	if stats.UsefulRate != 0.5 {
		t.Errorf("UsefulRate = %f, want 0.5", stats.UsefulRate)
	}
	if stats.NoiseRate != 0.25 {
		t.Errorf("NoiseRate = %f, want 0.25", stats.NoiseRate)
	}
	if stats.WrongRate != 0.25 {
		t.Errorf("WrongRate = %f, want 0.25", stats.WrongRate)
	}
}
