package sensitivity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// assumedSigma is the pass-to-pass scatter of corner time used until a plan on this car and
// track has measured one: the 28 ms the engine assumes (engine/paddock_engine/stats).
const assumedSigma = 0.028

// The car, how many corners its track has, and the scatter the newest plan on this car and
// track was made with.
const sessionSQL = `
SELECT s.car_id,
       (SELECT count(*) FROM corner c WHERE c.track_id = s.track_id),
       COALESCE((SELECT (e.mde->>'sigma')::float8
                 FROM experiment e JOIN hypothesis h ON h.id = e.hypothesis_id
                 WHERE h.car_id = s.car_id AND h.track_id = s.track_id
                   AND e.mde->>'sigma' IS NOT NULL
                 ORDER BY e.id DESC LIMIT 1), $2)
FROM session s WHERE s.id = $1`

// Proposals ranks the experiments the session's latest diagnosis names by what the car's
// sensitivity profile expects of them. The profile is rebuilt first, so an experiment that
// finished a moment ago already counts.
func (s *Service) Proposals(ctx context.Context, sessionID int64) (contracts.Proposals, error) {
	var carID int64
	var corners int
	var sigma float64
	err := s.database.QueryRowContext(ctx, sessionSQL, sessionID, assumedSigma).Scan(&carID, &corners, &sigma)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.Proposals{}, fmt.Errorf("session %d: %w", sessionID, experiments.ErrNotFound)
	}
	if err != nil {
		return contracts.Proposals{}, fmt.Errorf("find session %d: %w", sessionID, err)
	}
	diagnosis, err := s.diagnoses.Latest(ctx, sessionID)
	if err != nil {
		return contracts.Proposals{}, err
	}
	profiles, err := s.Profiles(ctx)
	if err != nil {
		return contracts.Proposals{}, err
	}
	request := proposeRequest(candidatesOf(diagnosis), carID, profiles)
	request.Sigma, request.Corners = sigma, max(corners, 1)
	proposals, err := s.engine.Propose(ctx, request)
	if err != nil {
		return contracts.Proposals{}, fmt.Errorf("rank experiments for session %d: %w", sessionID, err)
	}
	return proposals, nil
}

func proposeRequest(candidates []contracts.ProposeCandidate, carID int64, profiles []contracts.SensitivityProfile) contracts.ProposeRequest {
	test := experiments.DefaultTest()
	request := contracts.ProposeRequest{
		Candidates: candidates, Effects: []contracts.SensitivityEffect{},
		Channel: test.Channel, Better: contracts.ProposeRequestBetter(test.Better), Rope: test.Rope,
		LapsPerBlock: experiments.DefaultLapsPerBlock,
	}
	for _, profile := range profiles {
		if int64(profile.CarId) == carID {
			request.Effects, request.Experiments = profile.Effects, profile.Experiments
		}
	}
	return request
}

// candidatesOf lists every change the diagnosis names once, most strongly supported
// hypothesis first, with the hypotheses it would test.
func candidatesOf(diagnosis contracts.Diagnosis) []contracts.ProposeCandidate {
	type change struct {
		parameter string
		clicks    int
	}
	index := map[change]int{}
	candidates := []contracts.ProposeCandidate{}
	for _, hypothesis := range diagnosis.Hypotheses {
		for _, c := range hypothesis.Candidates {
			key := change{c.Parameter, c.Clicks}
			at, seen := index[key]
			if !seen {
				at = len(candidates)
				index[key] = at
				candidates = append(candidates, contracts.ProposeCandidate{
					Parameter: c.Parameter, Label: c.Label, Clicks: c.Clicks, Why: c.Why,
					Hypotheses: []string{},
				})
			}
			candidates[at].Hypotheses = append(candidates[at].Hypotheses, hypothesis.Key)
		}
	}
	return candidates
}
