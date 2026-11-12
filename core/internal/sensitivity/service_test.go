package sensitivity

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
)

func TestProfilesAreBuiltFromFinishedExperimentsAndStored(t *testing.T) {
	f := newFixture(t, "sensitivity_profiles")
	softer := f.finished(t, entry{parameter: "ARB_FRONT", clicks: -1}, -0.03)
	f.finished(t, entry{parameter: "ARB_REAR", clicks: 1, state: "rejected"}, 0.02)

	profiles, err := f.service.Profiles(t.Context())

	if err != nil {
		t.Fatalf("Profiles: %v", err)
	}
	if len(profiles) != 1 || len(profiles[0].Effects) != 2 {
		t.Fatalf("profiles = %+v, want the sample car with two effects", profiles)
	}
	got := profiles[0]
	wantLabels := contracts.SensitivityProfileLabels{"ARB_FRONT": "ARB Front", "ARB_REAR": "ARB Rear"}
	if got.CarId != int(f.carID) || got.Experiments != 2 || got.ModelVersion != fakeModel ||
		!reflect.DeepEqual(got.Labels, wantLabels) {
		t.Errorf("profile = %+v", got)
	}
	request := f.engine.asked[0]
	wantFirst := contracts.Measurement{
		ExperimentId: softer, Parameter: "ARB_FRONT", Clicks: -1, Channel: "corner_time_s",
		CornerClass: contracts.MeasurementCornerClassAll, Estimate: -0.03, CiLow: -0.04, CiHigh: -0.02, NPasses: 84,
	}
	if len(f.engine.asked) != 1 || len(request.Measurements) != 2 || request.Measurements[0] != wantFirst {
		t.Errorf("engine was asked %+v", f.engine.asked)
	}
	if request.Level != 0.9 || request.Ropes["corner_time_s"] != 0.005 || request.Ropes["rear_slip_peak_deg"] != 0.5 {
		t.Errorf("level %v and bands %v, want the ones the experiments are judged with", request.Level, request.Ropes)
	}
	var direction, ids, model string
	var clicks int
	var prior bool
	f.scan(t, `SELECT e.direction, e.clicks, e.is_prior, e.experiment_ids::text, e.model_version
	           FROM sensitivity_effect e JOIN setup_parameter p ON p.id = e.parameter_id
	           WHERE p.name = 'ARB_FRONT'`, nil, &direction, &clicks, &prior, &ids, &model)
	if direction != "decrease" || clicks != 1 || prior || ids != fmt.Sprintf("{%d}", softer) || model != fakeModel {
		t.Errorf("stored row = %s %d prior=%v %s %s", direction, clicks, prior, ids, model)
	}
}

func TestRebuildingReplacesTheStoredRows(t *testing.T) {
	f := newFixture(t, "sensitivity_rebuild")
	f.finished(t, entry{parameter: "ARB_FRONT", clicks: -1}, -0.03)

	for range 2 {
		if _, err := f.service.Profiles(t.Context()); err != nil {
			t.Fatalf("Profiles: %v", err)
		}
	}

	var rows int
	f.scan(t, `SELECT count(*) FROM sensitivity_effect`, nil, &rows)
	if rows != 1 {
		t.Errorf("%d stored rows after two rebuilds, want 1", rows)
	}
}

func TestExperimentsThatSayNothingAboutOneItemAreLeftOut(t *testing.T) {
	tests := []struct {
		name  string
		entry entry
	}{
		{name: "still being driven", entry: entry{parameter: "ARB_FRONT", clicks: -1, state: "running"}},
		{name: "aborted", entry: entry{parameter: "ARB_FRONT", clicks: -1, state: "aborted"}},
		{name: "a re-check of the champion", entry: entry{parameter: "ARB_FRONT", clicks: -1, kind: "recheck"}},
		{name: "judged at another level", entry: entry{parameter: "ARB_FRONT", clicks: -1, level: 0.95}},
		{name: "no change", entry: entry{parameter: "ARB_FRONT", clicks: 0}},
	}
	f := newFixture(t, "sensitivity_left_out")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.exec(t, `DELETE FROM experiment`)
			f.engine.asked = nil
			f.result(t, f.add(t, tt.entry), -0.03)

			profiles, err := f.service.Profiles(t.Context())

			if err != nil {
				t.Fatalf("Profiles: %v", err)
			}
			if len(f.engine.asked[0].Measurements) != 0 || len(profiles[0].Effects) != 0 {
				t.Errorf("engine was asked %+v", f.engine.asked)
			}
		})
	}
}

func TestAnotherCarsEffectArrivesAsAPrior(t *testing.T) {
	f := newFixture(t, "sensitivity_prior")
	f.exec(t, `UPDATE car SET drivetrain = 'rwd'`)
	f.exec(t, `INSERT INTO car (ac_id, name, drivetrain) VALUES ('other_car', 'Another Car', 'rwd')`)
	// The other car's front bar is named differently and its menu runs the other way.
	f.exec(t, `INSERT INTO setup_parameter (car_id, name, label, min, max, step, shown_scale, role, axle)
	           SELECT id, 'ARB_F', 'Front bar', 0, 5, 1, -1, 'arb', 'F' FROM car WHERE ac_id = 'other_car'`)
	f.finished(t, entry{parameter: "ARB_FRONT", clicks: -1}, -0.03)

	profiles, err := f.service.Profiles(t.Context())

	if err != nil {
		t.Fatalf("Profiles: %v", err)
	}
	other := profiles[0] // "Another Car" sorts before the Mazda
	if len(profiles) != 2 || other.CarName != "Another Car" || len(other.Effects) != 1 {
		t.Fatalf("profiles = %+v", profiles)
	}
	prior := other.Effects[0]
	if !prior.IsPrior || prior.PriorSource != profiles[1].CarName || prior.Parameter != "ARB_F" || prior.Clicks != 1 {
		t.Errorf("prior = %+v, want the Mazda's effect on ARB_F, one click the other way", prior)
	}
	if len(f.engine.asked) != 3 {
		t.Fatalf("engine was asked %d times, want once per car and once more for the borrower", len(f.engine.asked))
	}
	lent := f.engine.asked[2].Transfers
	if len(lent) != 1 || !lent[0].SameDrivetrain || lent[0].Estimate != -0.03 || lent[0].NPasses != 84 {
		t.Errorf("transfers = %+v", lent)
	}
	var lender int64
	var direction string
	f.scan(t, `SELECT prior_source_car_id, direction FROM sensitivity_effect WHERE is_prior`, nil, &lender, &direction)
	if lender != f.carID || direction != "increase" {
		t.Errorf("stored prior: lender %d direction %s, want car %d increase", lender, direction, f.carID)
	}
}

func TestOnlyACarsOneItemForAJobLendsOrBorrows(t *testing.T) {
	bar := knob{name: "ARB_FRONT", role: "arb", axle: "F", shownScale: 1}
	bump := knob{name: "BUMP_LF", role: "damper", kind: "slowbump", axle: "F", shownScale: 1}
	effect := func(parameter string) contracts.SensitivityResponse {
		return contracts.SensitivityResponse{Effects: []contracts.SensitivityEffect{{Parameter: parameter, Clicks: 1}}}
	}
	tests := []struct {
		name      string
		source    car
		target    car
		parameter string
		want      []string
	}{
		{name: "one item each", source: car{id: 1, knobs: []knob{bar}}, parameter: "ARB_FRONT",
			target: car{id: 2, knobs: []knob{{name: "ARB_F", role: "arb", axle: "F"}}}, want: []string{"ARB_F"}},
		{name: "a left and a right on the target", source: car{id: 1, knobs: []knob{bump}}, parameter: "BUMP_LF",
			target: car{id: 2, knobs: []knob{
				{name: "BUMP_L", role: "damper", kind: "slowbump", axle: "F"},
				{name: "BUMP_R", role: "damper", kind: "slowbump", axle: "F"},
			}}, want: []string{}},
		{name: "another kind of damper", source: car{id: 1, knobs: []knob{bump}}, parameter: "BUMP_LF",
			target: car{id: 2, knobs: []knob{{name: "REBOUND", role: "damper", kind: "slowrebound", axle: "F"}}},
			want:   []string{}},
		{name: "the other axle", source: car{id: 1, knobs: []knob{bar}}, parameter: "ARB_FRONT",
			target: car{id: 2, knobs: []knob{{name: "ARB_R", role: "arb", axle: "R"}}}, want: []string{}},
		{name: "an item the catalog could not classify", source: car{id: 1, knobs: []knob{{name: "ODD"}}},
			parameter: "ODD", target: car{id: 2, knobs: []knob{{name: "ALSO_ODD"}}}, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			measured := map[int64]contracts.SensitivityResponse{tt.source.id: effect(tt.parameter)}

			got := []string{}
			for _, lent := range transfersFor(tt.target, []car{tt.source, tt.target}, measured) {
				got = append(got, lent.Parameter)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lent to %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnEngineThatIsDownIsReportedAndNothingIsStored(t *testing.T) {
	f := newFixture(t, "sensitivity_engine_down")
	f.engine.failure = fmt.Errorf("connection refused: %w", engine.ErrUnavailable)

	_, err := f.service.Profiles(t.Context())

	if !errors.Is(err, engine.ErrUnavailable) {
		t.Errorf("Profiles error = %v, want engine.ErrUnavailable", err)
	}
}
