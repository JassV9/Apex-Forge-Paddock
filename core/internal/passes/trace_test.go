package passes

import (
	"math"
	"reflect"
	"testing"
)

func TestKeptIndices(t *testing.T) {
	tests := []struct {
		name      string
		n         int
		maxPoints int
		want      []int
		wantStep  int
	}{
		{name: "fewer samples than asked: all", n: 4, maxPoints: 50, want: []int{0, 1, 2, 3}, wantStep: 1},
		{name: "exactly as many: all", n: 5, maxPoints: 5, want: []int{0, 1, 2, 3, 4}, wantStep: 1},
		{name: "even thinning ends on the last", n: 9, maxPoints: 5, want: []int{0, 2, 4, 6, 8}, wantStep: 2},
		{name: "the last sample is always kept", n: 10, maxPoints: 5, want: []int{0, 3, 6, 9}, wantStep: 3},
		{name: "uneven: last added without passing the limit", n: 11, maxPoints: 5, want: []int{0, 3, 6, 9, 10}, wantStep: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, step := keptIndices(tt.n, tt.maxPoints)

			if !reflect.DeepEqual(got, tt.want) || step != tt.wantStep {
				t.Errorf("keptIndices(%d, %d) = %v step %d, want %v step %d", tt.n, tt.maxPoints, got, step, tt.want, tt.wantStep)
			}
			if len(got) > tt.maxPoints {
				t.Errorf("%d points, more than the %d asked for", len(got), tt.maxPoints)
			}
		})
	}
}

func TestKeptIndicesNeverExceedTheLimit(t *testing.T) {
	for n := 1; n < 700; n++ {
		for _, limit := range []int{50, 97, 333} {
			got, _ := keptIndices(n, limit)
			if len(got) > limit || got[0] != 0 || got[len(got)-1] != n-1 {
				t.Fatalf("n=%d limit=%d: %d points from %d to %d", n, limit, len(got), got[0], got[len(got)-1])
			}
		}
	}
}

// The cases follow engine/paddock_engine/features/axles.py.
func TestAxleSlip(t *testing.T) {
	tests := []struct {
		name                         string
		angleA, angleB, loadA, loadB float64
		want                         float64
	}{
		{name: "equal loads: the plain mean", angleA: 2, angleB: 4, loadA: 3000, loadB: 3000, want: 3},
		{name: "the loaded tyre weighs more", angleA: 2, angleB: 4, loadA: 3000, loadB: 1000, want: 2.5},
		{name: "sign of the angle does not matter", angleA: -2, angleB: 4, loadA: 3000, loadB: 1000, want: 2.5},
		{name: "a negative load counts as none", angleA: 2, angleB: 4, loadA: 3000, loadB: -500, want: 2},
		{name: "an unloaded axle: the plain mean", angleA: 1, angleB: 5, loadA: 0.4, loadB: 0.4, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := axleSlip(tt.angleA, tt.angleB, tt.loadA, tt.loadB); math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("axleSlip = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildTraceKeepsChannelsInStep(t *testing.T) {
	rows := make([]sampleRow, 120)
	for i := range rows {
		rows[i] = sampleRow{T: float64(i) / 60, D: 100 + float64(i), V: 50, SaFl: 1, SaFr: 3, LoadFl: 1, LoadFr: 1}
	}

	got := buildTrace(rows, 50)

	if got.NRecorded != 120 || got.N != len(got.Columns.T) || got.N > 50 || got.Step != 3 {
		t.Fatalf("trace has n=%d of %d, step %d", got.N, got.NRecorded, got.Step)
	}
	for name, column := range map[string][]float64{"d": got.Columns.D, "rh_r": got.Columns.RhR, "front": got.Axles.FrontSlipDeg} {
		if len(column) != got.N {
			t.Errorf("column %s has %d values, want %d", name, len(column), got.N)
		}
	}
	if got.Columns.D[1] != 103 || got.Columns.D[got.N-1] != 219 || got.Axles.FrontSlipDeg[0] != 2 {
		t.Errorf("d = %v…%v, front slip %v", got.Columns.D[1], got.Columns.D[got.N-1], got.Axles.FrontSlipDeg[0])
	}
}
