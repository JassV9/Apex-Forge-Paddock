package discovery

import (
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// responseLevel is the level of the response analysis's intervals: core sends no level, and
// the engine's default is 0.9 (docs/adr/0031-corner-phases-speed-regimes-and-response.md).
const responseLevel = 0.9

// measured is one closed step: its experiment as the ledger has it and its response cells.
type measured struct {
	clicks int
	exp    contracts.Experiment
	cells  []contracts.ResponseEffect
}

// observableKey names one observable across the steps.
type observableKey struct {
	source, metric, phase, regime, role string
}

// curveRequest gathers every observable the steps measured, each with its effect at every
// step: the verdict's channels (time first; block-level intervals, ADR 0029) per speed class,
// and the response cells the item's parameter row expects to move (ADR 0031) per phase and
// regime. An observable a step did not measure simply has no point at that step.
func curveRequest(parameter string, steps []measured) contracts.DiscoveryCurveRequest {
	request := contracts.DiscoveryCurveRequest{Parameter: parameter, Observables: []contracts.CurveObservable{}}
	index := map[observableKey]int{}
	add := func(key observableKey, observable contracts.CurveObservable, point contracts.CurveStep) {
		at, seen := index[key]
		if !seen {
			observable.Steps = []contracts.CurveStep{}
			request.Observables = append(request.Observables, observable)
			at = len(request.Observables) - 1
			index[key] = at
		}
		request.Observables[at].Steps = append(request.Observables[at].Steps, point)
	}
	for _, step := range steps {
		for _, result := range step.exp.Results {
			key, observable, point := verdictPoint(step, result)
			add(key, observable, point)
		}
		for _, cell := range step.cells {
			if cell.Estimate == nil || cell.CiLow == nil || cell.CiHigh == nil {
				continue
			}
			key, observable, point := responsePoint(step.clicks, cell)
			add(key, observable, point)
		}
	}
	return request
}

// verdictPoint is one channel of a step's verdict. The primary channel over all corners is
// the repeated confidence interval of the step's look; the others are at the success test's
// level. Its margin is the verdict's for the primary channel and the guardrail's tolerance for
// a guardrail.
func verdictPoint(step measured, result contracts.ExperimentResult) (observableKey, contracts.CurveObservable, contracts.CurveStep) {
	role := string(result.Role)
	key := observableKey{"verdict", result.Channel, "corner", string(result.CornerClass), role}
	observable := contracts.CurveObservable{
		Metric: result.Channel, Phase: contracts.CurveObservablePhaseCorner,
		Regime: contracts.CurveObservableRegime(result.CornerClass), Source: contracts.CurveObservableSourceVerdict,
		Role: &role, Margin: verdictMargin(step.exp, result),
	}
	point := contracts.CurveStep{
		Clicks: step.clicks, Estimate: result.Estimate, CiLow: result.CiLow, CiHigh: result.CiHigh,
		Level: step.exp.SuccessTest.Level,
	}
	primaryOverAll := result.Role == contracts.ExperimentResultRolePrimary &&
		result.CornerClass == contracts.ExperimentResultCornerClassAll
	if verdict := step.exp.Verdict; primaryOverAll && verdict != nil && verdict.Look != nil {
		point.Look = verdict.Look
	}
	return key, observable, point
}

func verdictMargin(exp contracts.Experiment, result contracts.ExperimentResult) *float64 {
	switch result.Role {
	case contracts.ExperimentResultRolePrimary:
		if exp.Verdict != nil && exp.Verdict.Margin != nil && *exp.Verdict.Margin > 0 {
			return exp.Verdict.Margin
		}
	case contracts.ExperimentResultRoleGuardrail:
		for _, guardrail := range exp.Guardrails {
			if guardrail.Channel == result.Channel && guardrail.Tolerance > 0 {
				tolerance := guardrail.Tolerance
				return &tolerance
			}
		}
	}
	return nil
}

// responsePoint is one response cell of a step.
func responsePoint(clicks int, cell contracts.ResponseEffect) (observableKey, contracts.CurveObservable, contracts.CurveStep) {
	key := observableKey{"response", cell.Metric, string(cell.Phase), string(cell.Regime), ""}
	expected := contracts.CurveObservableExpected(cell.Expected)
	observable := contracts.CurveObservable{
		Metric: cell.Metric, Phase: contracts.CurveObservablePhase(cell.Phase),
		Regime: contracts.CurveObservableRegime(cell.Regime), Source: contracts.CurveObservableSourceResponse,
		Expected: &expected,
	}
	if cell.Margin != nil && *cell.Margin > 0 {
		observable.Margin = cell.Margin
	}
	point := contracts.CurveStep{
		Clicks: clicks, Estimate: *cell.Estimate, CiLow: *cell.CiLow, CiHigh: *cell.CiHigh, Level: responseLevel,
	}
	return key, observable, point
}

// expectedCells keeps the response cells the item's parameter row lists: the curve is
// classified for what the item is expected to move, not for all 344 cells, most of which would
// move by chance at 90 % (ADR 0031, known limits).
func expectedCells(effects []contracts.ResponseEffect) []contracts.ResponseEffect {
	cells := []contracts.ResponseEffect{}
	for _, effect := range effects {
		if effect.Expected != contracts.ResponseResponseEffectsElemExpectedNone {
			cells = append(cells, effect)
		}
	}
	return cells
}
