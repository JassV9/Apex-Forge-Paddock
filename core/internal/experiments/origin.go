package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// origin is where a hypothesis came from and what it rested on when it was written down.
type origin struct {
	source       contracts.ExperimentSource
	phase        *string
	cornerClass  *string
	evidence     string // JSON list of evidence ids
	alternatives string // JSON list of contracts.Alternative
}

func humanOrigin() origin {
	return origin{source: contracts.ExperimentSourceHuman, evidence: "[]", alternatives: "[]"}
}

func rulesOrigin() origin {
	return origin{source: contracts.ExperimentSourceRules, evidence: "[]", alternatives: "[]"}
}

// originOf is the human's own hypothesis when no key is given. With a key it is the
// hypothesis of that key in the session's latest diagnosis: its evidence and the alternatives
// it was shown with go into the ledger, so a later reader sees what was known at the time.
func originOf(ctx context.Context, q querier, sessionID int64, key *string) (origin, error) {
	if key == nil {
		return humanOrigin(), nil
	}
	var body []byte
	err := q.QueryRowContext(ctx,
		`SELECT body FROM diagnosis WHERE session_id = $1 ORDER BY id DESC LIMIT 1`, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return origin{}, fmt.Errorf("session %d has no diagnosis to take hypothesis %s from: %w", sessionID, *key, ErrInvalid)
	}
	if err != nil {
		return origin{}, fmt.Errorf("read diagnosis of session %d: %w", sessionID, err)
	}
	var diagnosis contracts.Diagnosis
	if err := json.Unmarshal(body, &diagnosis); err != nil {
		return origin{}, fmt.Errorf("decode diagnosis of session %d: %w", sessionID, err)
	}
	for _, hypothesis := range diagnosis.Hypotheses {
		if hypothesis.Key == *key {
			return originFrom(hypothesis)
		}
	}
	return origin{}, fmt.Errorf("the latest diagnosis of session %d has no hypothesis %s: %w", sessionID, *key, ErrInvalid)
}

func originFrom(hypothesis contracts.Hypothesis) (origin, error) {
	evidence, err := json.Marshal(hypothesis.Evidence)
	if err != nil {
		return origin{}, fmt.Errorf("encode evidence of %s: %w", hypothesis.Key, err)
	}
	alternatives, err := json.Marshal(hypothesis.Alternatives)
	if err != nil {
		return origin{}, fmt.Errorf("encode alternatives of %s: %w", hypothesis.Key, err)
	}
	phase, cornerClass := string(hypothesis.Phase), string(hypothesis.CornerClass)
	return origin{
		source: contracts.ExperimentSourceRules, phase: &phase, cornerClass: &cornerClass,
		evidence: string(evidence), alternatives: string(alternatives),
	}, nil
}
