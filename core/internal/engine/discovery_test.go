package engine

import (
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestCurvePostsTheStepsAndDecodesTheShapes(t *testing.T) {
	var got contracts.DiscoveryCurveRequest
	server := engineStub(t, "/discovery/curve", `{"parameter":"ARB_REAR","level":0.9,
		"engine_version":"0.2.0","curves":[{"metric":"rear_slip_deg","phase":"mid","regime":"all",
		"source":"response","unit":"deg","shape":"linear","clicks":[-1,1,2],
		"slope":{"estimate":0.1,"ci_low":0.08,"ci_high":0.12},"reason":"straight"}]}`, &got)
	request := contracts.DiscoveryCurveRequest{Parameter: "ARB_REAR", Observables: []contracts.CurveObservable{{
		Metric: "rear_slip_deg", Phase: contracts.CurveObservablePhaseMid, Regime: contracts.CurveObservableRegimeAll,
		Source: contracts.CurveObservableSourceResponse,
		Steps:  []contracts.CurveStep{{Clicks: 1, Estimate: 0.1, CiLow: 0.05, CiHigh: 0.15, Level: 0.9}},
	}}}

	reply, err := New(server.URL).Curve(t.Context(), request)

	if err != nil {
		t.Fatalf("Curve: %v", err)
	}
	if len(got.Observables) != 1 || got.Observables[0].Steps[0].Clicks != 1 {
		t.Errorf("engine received %+v", got)
	}
	if len(reply.Curves) != 1 || reply.Curves[0].Shape != contracts.CurveResultShapeLinear || reply.Curves[0].Slope.Estimate != 0.1 {
		t.Errorf("reply = %+v", reply)
	}
}

func TestResponsePostsThePassesAndDecodesTheCells(t *testing.T) {
	var got contracts.ResponseRequest
	server := engineStub(t, "/response", `{"regimes":{"method":"fixed","breaks_kmh":[100,150]},
		"corners":[],"unavailable":[],"passes_used":2,"passes_excluded":0,"notes":[],
		"effects":[{"metric":"rear_slip_deg","phase":"mid","regime":"all","kind":"direct",
		"finding":"increase","estimate":0.1,"ci_low":0.05,"ci_high":0.15,"margin":0.05,
		"n_passes":2,"corners":1,"expected":"direct"}]}`, &got)
	request := contracts.ResponseRequest{
		TrackLengthM: 4000,
		Corners:      []contracts.ResponseCorner{{Number: 1, ZoneStartM: 0, StartM: 10, EndM: 90, ZoneEndM: 100}},
		Sessions:     []contracts.ResponseSession{{Session: 3, ParquetDir: "/data/3"}},
		Passes:       []contracts.ResponsePass{},
	}

	reply, err := New(server.URL).Response(t.Context(), request)

	if err != nil {
		t.Fatalf("Response: %v", err)
	}
	if got.TrackLengthM != 4000 || got.Sessions[0].ParquetDir != "/data/3" {
		t.Errorf("engine received %+v", got)
	}
	if len(reply.Effects) != 1 || reply.Effects[0].Expected != contracts.ResponseResponseEffectsElemExpectedDirect {
		t.Errorf("reply = %+v", reply)
	}
}
