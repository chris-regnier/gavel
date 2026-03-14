package feedback

// Stats holds aggregate feedback statistics.
type Stats struct {
	Total      int     `json:"total"`
	Useful     int     `json:"useful"`
	Noise      int     `json:"noise"`
	Wrong      int     `json:"wrong"`
	UsefulRate float64 `json:"useful_rate"`
	NoiseRate  float64 `json:"noise_rate"`
	WrongRate  float64 `json:"wrong_rate"`
}

// ComputeStats computes aggregate statistics from feedback entries.
func ComputeStats(entries []Entry) Stats {
	s := Stats{Total: len(entries)}
	for _, e := range entries {
		switch e.Verdict {
		case VerdictUseful:
			s.Useful++
		case VerdictNoise:
			s.Noise++
		case VerdictWrong:
			s.Wrong++
		}
	}
	if s.Total > 0 {
		n := float64(s.Total)
		s.UsefulRate = float64(s.Useful) / n
		s.NoiseRate = float64(s.Noise) / n
		s.WrongRate = float64(s.Wrong) / n
	}
	return s
}
