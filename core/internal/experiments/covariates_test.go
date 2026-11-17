package experiments

import (
	"database/sql"
	"maps"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestCovariatesAreWhatThePassRecorded(t *testing.T) {
	known := func(value float64) sql.NullFloat64 { return sql.NullFloat64{Float64: value, Valid: true} }
	missing := sql.NullFloat64{}
	cases := []struct {
		name                     string
		fuel, tyreCore, ersStart sql.NullFloat64
		want                     contracts.EvaluatePassCovariates
	}{
		{"hybrid", known(31.5), known(88), known(0.72),
			contracts.EvaluatePassCovariates{"time_in_session_s": 600, "fuel_l": 31.5, "tyre_core_c": 88, "ers_charge_start": 0.72}},
		{"hybrid with a flat battery", known(31.5), known(88), known(0),
			contracts.EvaluatePassCovariates{"time_in_session_s": 600, "fuel_l": 31.5, "tyre_core_c": 88, "ers_charge_start": 0}},
		{"no battery", known(31.5), known(88), missing,
			contracts.EvaluatePassCovariates{"time_in_session_s": 600, "fuel_l": 31.5, "tyre_core_c": 88}},
		{"nothing recorded", missing, missing, missing,
			contracts.EvaluatePassCovariates{"time_in_session_s": 600}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := contracts.EvaluatePassCovariates{}

			addCovariates(got, c.fuel, c.tyreCore, c.ersStart, 600)

			if !maps.Equal(got, c.want) {
				t.Errorf("covariates = %v, want %v", got, c.want)
			}
		})
	}
}
