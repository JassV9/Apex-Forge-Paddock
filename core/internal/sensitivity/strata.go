package sensitivity

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/launcher"
)

// stratum is what an experiment was measured under, beside its item and clicks: what may change
// what one click does (docs/adr/0033-stratified-vehicle-sensitivity-profile.md). The engine never
// pools measurements of different strata into one row.
type stratum struct {
	track, trackName, driver, hint string
	baselineValue, clickStep       *float64
}

// hintSpread is the widest spread of planning shares an experiment's blocks may have and still
// be one hint: the agent rounds nothing, so anything wider was driven at different hints.
const hintSpread = 0.005

// testCopyName is how a layout's _paddock copy is named (tools/prepare_test_layout.py).
const testCopyName = " (Paddock test)"

// trackKey is the game track and layout, a _paddock test copy counted as its layout
// (docs/adr/0027-layout-variants.md): the AI drives the copy, a person the original.
func trackKey(acID, layout, name string) (string, string) {
	if original, copied := strings.CutSuffix(layout, launcher.TestCopySuffix); copied && original != "" {
		layout = original
		name = strings.TrimSuffix(name, testCopyName)
	}
	return acID + "/" + layout, name
}

// hintBucket is the AI's planning share of the car's tyre hint to the nearest 0.01 (ADR 0028):
// the experiment's pinned share, else what its blocks' run context recorded. A human has none;
// an experiment driven before run context was recorded is unknown; blocks driven at different
// shares are mixed and pool with nothing else.
func hintBucket(driver string, pinned, low, high sql.NullFloat64) string {
	switch {
	case driver != "ai":
		return "none"
	case pinned.Valid:
		return share(pinned.Float64)
	case !low.Valid || !high.Valid:
		return "unknown"
	case high.Float64-low.Float64 > hintSpread:
		return fmt.Sprintf("mixed %s-%s", share(low.Float64), share(high.Float64))
	default:
		return share((low.Float64 + high.Float64) / 2)
	}
}

func share(value float64) string {
	return fmt.Sprintf("%.2f", math.Round(value*100)/100)
}

func optional(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

// sessionTrack is the track key of the session's track.
func sessionTrack(ctx context.Context, database *sql.DB, sessionID int64) (string, error) {
	var acID, layout, name string
	err := database.QueryRowContext(ctx, `SELECT t.ac_id, t.layout, t.name FROM session s
	    JOIN track t ON t.id = s.track_id WHERE s.id = $1`, sessionID).Scan(&acID, &layout, &name)
	if err != nil {
		return "", fmt.Errorf("track of session %d: %w", sessionID, err)
	}
	key, _ := trackKey(acID, layout, name)
	return key, nil
}

// onTrack keeps the rows measured on the session's track, by `driver` when one is named: a
// proposal or a base setup for this track rests on this track's measurements, never on another
// track's or on the all-track scope.
func onTrack(effects []contracts.SensitivityEffect, track, driver string) []contracts.SensitivityEffect {
	kept := []contracts.SensitivityEffect{}
	for _, effect := range effects {
		if effect.Track == track && (driver == "" || string(effect.Driver) == driver) {
			kept = append(kept, effect)
		}
	}
	return kept
}
