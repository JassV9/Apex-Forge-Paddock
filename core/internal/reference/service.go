// Package reference compares a driver with AC's AI on the same car, track and setup
// (docs/adr/0019-ai-reference-driver.md): per corner, where the driver loses time the AI
// does not, and which technique metrics differ. Every number is computed here from stored
// passes; nothing is written.
package reference

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// ErrNotFound means the session does not exist, or has no valid pass on the asked setup.
var ErrNotFound = errors.New("not found")

// Service answers the reference routes from PostgreSQL.
type Service struct {
	database *sql.DB
}

// NewService reads passes and setups from database.
func NewService(database *sql.DB) *Service {
	return &Service{database: database}
}

// Session compares every corner of a session on one of its setups (empty: the setup of
// its latest pass) with the AI's passes on the same car and track.
func (s *Service) Session(ctx context.Context, sessionID int64, setupHash string) (contracts.SessionReference, error) {
	info, err := s.session(ctx, sessionID)
	if err != nil {
		return contracts.SessionReference{}, err
	}
	own, err := s.passes(ctx, sessionPassesSQL, sessionID)
	if err != nil {
		return contracts.SessionReference{}, fmt.Errorf("session %d: %w", sessionID, err)
	}
	driven, latest := sessionSetups(own)
	if setupHash == "" {
		setupHash = latest
	} else if !drove(driven, setupHash) {
		return contracts.SessionReference{}, fmt.Errorf("session %d has no valid pass on setup %s: %w", sessionID, setupHash, ErrNotFound)
	}
	in, err := s.input(ctx, info, sessionID, setupHash)
	if err != nil {
		return contracts.SessionReference{}, err
	}
	in.Human = onSetup(own, setupHash)
	reference := contracts.SessionReference{
		SessionId: int(sessionID), Car: info.car, Track: info.track, Layout: info.layout,
		Setups: driven, MarginFloorS: MarginFloorS, AiPasses: len(in.AI), Corners: Compare(in),
	}
	if setupHash != "" {
		reference.SetupHash = &setupHash
	}
	return reference, nil
}

// input loads the AI's passes, the car's setups and the corner names.
func (s *Service) input(ctx context.Context, info sessionInfo, sessionID int64, setupHash string) (Input, error) {
	ai, err := s.passes(ctx, aiPassesSQL, info.carID, info.trackID, sessionID)
	if err != nil {
		return Input{}, fmt.Errorf("AI passes for session %d: %w", sessionID, err)
	}
	values, err := s.setupValues(ctx, info.carID)
	if err != nil {
		return Input{}, err
	}
	names, err := s.cornerNames(ctx, info.trackID)
	if err != nil {
		return Input{}, err
	}
	return Input{AI: ai, SetupHash: setupHash, Values: values, Names: names}, nil
}

// ForSetup returns the AI's times per corner on exactly one setup of a car on a track
// layout, by game ids. Unknown cars, tracks and setups give no corners, not an error.
func (s *Service) ForSetup(ctx context.Context, car, track, layout, setupHash string) (contracts.AiReference, error) {
	reference := contracts.AiReference{Car: car, Track: track, Layout: layout, SetupHash: setupHash,
		Corners: []contracts.AiCornerTime{}}
	carID, trackID, ok, err := s.carAndTrack(ctx, car, track, layout)
	if err != nil || !ok {
		return reference, err
	}
	ai, err := s.passes(ctx, aiSetupPassesSQL, carID, trackID, setupHash)
	if err != nil {
		return reference, fmt.Errorf("AI passes of %s on %s %s: %w", car, track, layout, err)
	}
	names, err := s.cornerNames(ctx, trackID)
	if err != nil {
		return reference, err
	}
	reference.Corners = Times(ai, names)
	return reference, nil
}

// sessionSetups counts the passes per setup, most first, and names the setup of the latest
// pass that has one ("" when none has).
func sessionSetups(passes []Pass) ([]contracts.ReferenceSetup, string) {
	counts, latest := map[string]int{}, ""
	for _, pass := range passes {
		if pass.SetupHash != "" {
			counts[pass.SetupHash]++
			latest = pass.SetupHash
		}
	}
	driven := make([]contracts.ReferenceSetup, 0, len(counts))
	for hash, n := range counts {
		driven = append(driven, contracts.ReferenceSetup{SetupHash: hash, Passes: n})
	}
	sort.Slice(driven, func(i, j int) bool {
		if driven[i].Passes != driven[j].Passes {
			return driven[i].Passes > driven[j].Passes
		}
		return driven[i].SetupHash < driven[j].SetupHash
	})
	return driven, latest
}

func drove(driven []contracts.ReferenceSetup, hash string) bool {
	for _, setup := range driven {
		if setup.SetupHash == hash {
			return true
		}
	}
	return false
}

func onSetup(passes []Pass, hash string) []Pass {
	var kept []Pass
	for _, pass := range passes {
		if pass.SetupHash == hash {
			kept = append(kept, pass)
		}
	}
	return kept
}
