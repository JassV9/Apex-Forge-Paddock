package experiments

import (
	"context"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// ReplayRequest is the evaluate request a finished experiment's verdict was judged on, built
// again from the ledger: the same passes, exclusions, off-track counts and look, with no
// extension left. The sensitivity profile sends it to the engine to re-derive the experiment
// with today's method (docs/adr/0033-stratified-vehicle-sensitivity-profile.md); nothing is
// stored here. An experiment whose plan is not fully driven, or whose features are pending, is
// refused with ErrConflict.
func (s *Service) ReplayRequest(ctx context.Context, id int64) (contracts.EvaluateRequest, error) {
	exp, err := loadRecord(ctx, s.database, id, false)
	if err != nil {
		return contracts.EvaluateRequest{}, err
	}
	request, err := evaluateRequest(ctx, s.database, exp)
	if err != nil {
		return contracts.EvaluateRequest{}, err
	}
	none := 0
	request.ExtensionsLeft = &none
	return request, nil
}
