package passes

import (
	"math"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func pass(seq, corner int, seconds float64, valid bool) contracts.PassRecord {
	return contracts.PassRecord{Seq: seq, Corner: corner, DurationS: seconds, Valid: valid}
}

func near(a *float64, b float64) bool {
	return a != nil && math.Abs(*a-b) < 1e-9
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		name       string
		records    []contracts.PassRecord
		wantBest   float64
		wantSeq    int
		wantMedian float64
		wantSpread float64 // < 0: no spread
		wantValid  int
	}{
		{
			name:     "one pass has no spread",
			records:  []contracts.PassRecord{pass(1, 4, 5.5, true)},
			wantBest: 5.5, wantSeq: 1, wantMedian: 5.5, wantSpread: -1, wantValid: 1,
		},
		{
			name:     "odd count: the middle time",
			records:  []contracts.PassRecord{pass(1, 4, 5.3, true), pass(5, 4, 5.1, true), pass(9, 4, 5.2, true)},
			wantBest: 5.1, wantSeq: 5, wantMedian: 5.2, wantSpread: 0.1, wantValid: 3,
		},
		{
			name:     "even count: the mean of the middle two",
			records:  []contracts.PassRecord{pass(1, 4, 6, true), pass(2, 4, 5, true), pass(3, 4, 8, true), pass(4, 4, 7, true)},
			wantBest: 5, wantSeq: 2, wantMedian: 6.5, wantSpread: math.Sqrt(5.0 / 3), wantValid: 4,
		},
		{
			name:     "an invalid pass never counts, however quick",
			records:  []contracts.PassRecord{pass(1, 4, 4.0, false), pass(2, 4, 5.0, true), pass(3, 4, 5.2, true)},
			wantBest: 5.0, wantSeq: 2, wantMedian: 5.1, wantSpread: math.Sqrt(0.02), wantValid: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(tt.records, map[int]string{4: "T4"})

			if len(got) != 1 {
				t.Fatalf("%d summaries, want 1", len(got))
			}
			summary := got[0]
			if summary.Corner != 4 || summary.Name != "T4" || summary.Passes != len(tt.records) || summary.ValidPasses != tt.wantValid {
				t.Errorf("summary = %+v", summary)
			}
			if !near(summary.BestS, tt.wantBest) || summary.BestSeq == nil || *summary.BestSeq != tt.wantSeq {
				t.Errorf("best = %v at %v, want %v at %d", summary.BestS, summary.BestSeq, tt.wantBest, tt.wantSeq)
			}
			if !near(summary.MedianS, tt.wantMedian) {
				t.Errorf("median = %v, want %v", summary.MedianS, tt.wantMedian)
			}
			if tt.wantSpread < 0 && summary.SpreadS != nil || tt.wantSpread >= 0 && !near(summary.SpreadS, tt.wantSpread) {
				t.Errorf("spread = %v, want %v", summary.SpreadS, tt.wantSpread)
			}
		})
	}
}

func TestSummarizeOrdersCornersAndLeavesTimesOutWithoutAValidPass(t *testing.T) {
	records := []contracts.PassRecord{pass(1, 3, 4, true), pass(2, 1, 9, false), pass(3, 2, 6, true)}

	got := Summarize(records, map[int]string{})

	if len(got) != 3 || got[0].Corner != 1 || got[1].Corner != 2 || got[2].Corner != 3 {
		t.Fatalf("corners = %+v, want 1, 2, 3", got)
	}
	first := got[0]
	if first.Passes != 1 || first.ValidPasses != 0 || first.BestS != nil || first.MedianS != nil || first.SpreadS != nil {
		t.Errorf("corner without a valid pass = %+v", first)
	}
	if first.Name != "" {
		t.Errorf("a corner the map does not name has the name %q", first.Name)
	}
}
