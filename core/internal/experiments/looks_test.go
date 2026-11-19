package experiments

import (
	"slices"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestDriverEventsAreTestedWhereTheyHappenWheneverEveryPassHasThem(t *testing.T) {
	metre := 120.0
	absolute := contracts.EvaluatePassMetrics{"brake_point_m": &metre, "throttle_pickup_point_m": nil}
	relative := contracts.EvaluatePassMetrics{"brake_dist_m": &metre}
	cases := []struct {
		name   string
		passes []contracts.EvaluatePassMetrics
		want   []string
	}{
		{"new features", []contracts.EvaluatePassMetrics{absolute, absolute},
			[]string{"brake_point_m", "throttle_pickup_point_m", "steer_corrections_per_s"}},
		{"one pass from an older engine", []contracts.EvaluatePassMetrics{absolute, relative},
			[]string{"brake_dist_m", "throttle_pickup_m", "steer_corrections_per_s"}},
		{"no passes", nil, driverChannels},
	}
	for _, c := range cases {
		passes := []contracts.EvaluatePass{}
		for _, metrics := range c.passes {
			passes = append(passes, contracts.EvaluatePass{Metrics: metrics})
		}
		if got := driverInputs(passes); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
