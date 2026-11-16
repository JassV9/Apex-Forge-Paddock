package reference_test

import (
	"math"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/reference"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// passes makes one pass of corner 1 per time, on a setup, with a brake point.
func passes(session int64, setup string, brakeM float64, times ...float64) []reference.Pass {
	made := make([]reference.Pass, len(times))
	for i, duration := range times {
		made[i] = reference.Pass{SessionID: session, Seq: i + 1, Corner: 1, DurationS: duration,
			SetupHash: setup, Metrics: map[string]float64{"brake_dist_m": brakeM, "v_min_kmh": 100}}
	}
	return made
}

func joined(lists ...[]reference.Pass) []reference.Pass {
	var all []reference.Pass
	for _, list := range lists {
		all = append(all, list...)
	}
	return all
}

var values = map[string]setups.Values{
	"mine":  {"ARB_FRONT": 3, "WING": 5, "CAMBER": -3},
	"near":  {"ARB_FRONT": 4, "WING": 5, "CAMBER": -3},
	"far":   {"ARB_FRONT": 1, "WING": 2, "CAMBER": -3},
	"other": {"ARB_FRONT": 9, "WING": 9, "CAMBER": -9},
}

func near(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

func TestCompareClassesEachCorner(t *testing.T) {
	ai := passes(57, "mine", 100, 5.000, 5.002, 4.998, 5.001, 5.300)
	tests := []struct {
		name        string
		human       []reference.Pass
		ai          []reference.Pass
		setup       string
		wantMatch   contracts.CornerReferenceMatch
		wantClass   contracts.CornerReferenceClass
		wantChanges int
		wantReason  string
	}{
		{name: "slower than the AI by more than its margin", human: passes(3, "mine", 120, 5.20, 5.10, 5.30),
			ai: ai, setup: "mine", wantMatch: "same", wantClass: "driver", wantReason: "0.199 s slower"},
		{name: "within the margin", human: passes(3, "mine", 100, 5.005, 4.99),
			ai: ai, setup: "mine", wantMatch: "same", wantClass: "car", wantReason: "within"},
		{name: "faster than the AI", human: passes(3, "mine", 100, 4.80),
			ai: ai, setup: "mine", wantMatch: "same", wantClass: "car", wantReason: "faster"},
		{name: "the nearest setup when the AI never drove this one",
			human: passes(3, "mine", 100, 5.2),
			ai:    joined(passes(57, "far", 100, 4, 4, 4), passes(57, "near", 100, 5, 5, 5)),
			setup: "mine", wantMatch: "nearest", wantClass: "driver", wantChanges: 1, wantReason: "1 items differ"},
		{name: "too few AI passes on any setup", human: passes(3, "mine", 100, 5.2),
			ai: joined(passes(57, "mine", 100, 5, 5), passes(57, "near", 100, 5)), setup: "mine",
			wantMatch: "none", wantClass: "no_reference", wantReason: "2 AI passes"},
		{name: "a setup core has no values of", human: passes(3, "unknown", 100, 5.2),
			ai: passes(57, "near", 100, 5, 5, 5), setup: "unknown", wantMatch: "none", wantClass: "no_reference"},
		{name: "a session without setup data", human: passes(3, "", 100, 5.2),
			ai: passes(57, "", 100, 5, 5, 5), setup: "", wantMatch: "none", wantClass: "no_reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reference.Compare(reference.Input{Human: tt.human, AI: tt.ai, SetupHash: tt.setup,
				Values: values, Names: map[int]string{1: "T1"}})

			if len(got) != 1 {
				t.Fatalf("%d corners, want 1", len(got))
			}
			corner := got[0]
			if corner.Match != tt.wantMatch || corner.Class != tt.wantClass || corner.Name != "T1" {
				t.Errorf("match %s class %s, want %s %s (%s)", corner.Match, corner.Class, tt.wantMatch, tt.wantClass, corner.Reason)
			}
			if corner.SetupChanges != nil && *corner.SetupChanges != tt.wantChanges {
				t.Errorf("setup changes = %d, want %d", *corner.SetupChanges, tt.wantChanges)
			}
			if !strings.Contains(corner.Reason, tt.wantReason) {
				t.Errorf("reason %q, want it to say %q", corner.Reason, tt.wantReason)
			}
		})
	}
}

func TestCompareNumbersCarryTheirCounts(t *testing.T) {
	human := passes(3, "mine", 120, 5.20, 5.10, 5.30)
	ai := passes(57, "mine", 100, 5.000, 5.002, 4.998, 5.001, 5.300)

	got := reference.Compare(reference.Input{Human: human, AI: ai, SetupHash: "mine", Values: values})[0]

	if got.HumanN != 3 || got.HumanBestS != 5.10 || got.HumanBestSeq != 2 || got.HumanMedianS != 5.20 {
		t.Errorf("driver side = n %d best %v (#%d) median %v", got.HumanN, got.HumanBestS, got.HumanBestSeq, got.HumanMedianS)
	}
	if got.AiN != 5 || !near(*got.AiMedianS, 5.001) || *got.AiBestSeq != 3 || *got.AiBestSessionId != 57 {
		t.Errorf("AI side = n %d median %v best #%d of %d", got.AiN, *got.AiMedianS, *got.AiBestSeq, *got.AiBestSessionId)
	}
	// One lap where the AI went off (5.300) must not widen the spread: the MAD is 0.001 s.
	if !near(*got.AiSpreadS, 1.4826*0.001) || *got.MarginS != reference.MarginFloorS {
		t.Errorf("spread %v margin %v", *got.AiSpreadS, *got.MarginS)
	}
	if !near(*got.DeltaMedianS, 0.199) || !near(*got.DeltaBestS, 0.099) {
		t.Errorf("deltas = median %v best %v", *got.DeltaMedianS, *got.DeltaBestS)
	}
	want := contracts.FeatureDifference{Metric: "brake_dist_m", Phase: "brake", HumanMedian: 120,
		AiMedian: 100, Difference: 20, Step: 5, HumanN: 3, AiN: 5}
	if len(got.Differences) != 1 || got.Differences[0] != want {
		t.Errorf("differences = %+v; v_min_kmh is equal and must not be named", got.Differences)
	}
}

func TestTheAIIsTheNewestSessionWithEnoughPasses(t *testing.T) {
	human := passes(3, "mine", 100, 5.2)
	older := passes(30, "mine", 100, 5.5, 5.5, 5.5)
	tests := []struct {
		name        string
		newer       []reference.Pass
		wantSession int
		wantMedian  float64
	}{
		{name: "the newer session has enough", newer: passes(57, "mine", 100, 5, 5, 5), wantSession: 57, wantMedian: 5},
		{name: "the newer session has too few", newer: passes(57, "mine", 100, 5, 5), wantSession: 30, wantMedian: 5.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ai := joined(older, tt.newer)

			got := reference.Compare(reference.Input{Human: human, AI: ai, SetupHash: "mine"})[0]

			if got.AiN != 3 || *got.AiBestSessionId != tt.wantSession || *got.AiMedianS != tt.wantMedian {
				t.Errorf("AI = n %d session %d median %v", got.AiN, *got.AiBestSessionId, *got.AiMedianS)
			}
		})
	}
}

func TestDifferencesAreTheLargestThreeInSteps(t *testing.T) {
	human := passes(3, "mine", 0, 5.2)
	ai := passes(57, "mine", 0, 5, 5, 5)
	human[0].Metrics = map[string]float64{"brake_dist_m": 10, "v_min_kmh": 90, "coast_s": 0.5,
		"throttle_pickup_m": 0, "trail_share": 0.5}
	for i := range ai {
		ai[i].Metrics = map[string]float64{"brake_dist_m": 0, "v_min_kmh": 100, "coast_s": 0.1,
			"throttle_pickup_m": -30, "trail_share": 0.52}
	}

	got := reference.Compare(reference.Input{Human: human, AI: ai, SetupHash: "mine"})[0].Differences

	var names []string
	for _, d := range got {
		names = append(names, d.Metric)
	}
	// In steps: throttle pickup 6, v_min 5, coast 4, brake 2; trail share is under its step.
	if strings.Join(names, ",") != "throttle_pickup_m,v_min_kmh,coast_s" {
		t.Errorf("differences = %v", names)
	}
}

func TestTimesListOnlyCornersWithEnoughPasses(t *testing.T) {
	ai := joined(passes(57, "s", 0, 5, 5.1, 5.2))
	for _, pass := range passes(57, "s", 0, 7, 7) {
		pass.Corner = 2
		ai = append(ai, pass)
	}

	got := reference.Times(ai, map[int]string{1: "T1"})

	if len(got) != 1 || got[0].Corner != 1 || got[0].N != 3 || got[0].Name != "T1" {
		t.Fatalf("times = %+v", got)
	}
	if !near(got[0].MedianS, 5.1) || !near(got[0].SpreadS, 1.4826*0.1) || got[0].MarginS != got[0].SpreadS {
		t.Errorf("corner 1 = %+v", got[0])
	}
}
