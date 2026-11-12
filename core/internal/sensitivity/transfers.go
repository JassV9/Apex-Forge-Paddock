package sensitivity

import "github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"

// transfersFor lists what the other cars measured on an item that does the same job as an
// item of the target. Item names differ between cars, jobs do not. Whether the borrowed
// number means anything here is the engine's call; it shrinks it and flags it as a prior.
func transfersFor(target car, cars []car, measured map[int64]contracts.SensitivityResponse) []contracts.Transfer {
	transfers := []contracts.Transfer{}
	for _, source := range cars {
		if source.id == target.id {
			continue
		}
		for _, effect := range measured[source.id].Effects {
			from, known := source.knob(effect.Parameter)
			if !known || len(sameJob(source, from)) != 1 {
				continue
			}
			if matches := sameJob(target, from); len(matches) == 1 {
				transfers = append(transfers, transfer(effect, source, target, from, matches[0]))
			}
		}
	}
	return transfers
}

// sameJob are a car's items with the role, kind and axle of `from`. An item the catalog
// could not classify has no role and matches nothing. Only a car's one item for a job
// lends or borrows: a left and a right item share a job, and one side's effect says
// nothing certain about the other's.
func sameJob(c car, from knob) []knob {
	matches := []knob{}
	for _, to := range c.knobs {
		if from.role != "" && to.role == from.role && to.kind == from.kind && to.axle == from.axle {
			matches = append(matches, to)
		}
	}
	return matches
}

// transfer restates another car's effect in the target's clicks. Clicks count steps of the
// raw value; when one car's menu runs the other way (a negative shown scale), a click up on
// one is a click down on the other.
func transfer(effect contracts.SensitivityEffect, source, target car, from, to knob) contracts.Transfer {
	return contracts.Transfer{
		Parameter:      to.name,
		SourceCar:      source.label,
		SameDrivetrain: source.drivetrain != "" && source.drivetrain == target.drivetrain,
		Clicks:         effect.Clicks * way(from.shownScale) * way(to.shownScale),
		Channel:        effect.Channel,
		CornerClass:    contracts.TransferCornerClass(effect.CornerClass),
		Estimate:       effect.Estimate,
		CiLow:          effect.CiLow,
		CiHigh:         effect.CiHigh,
		NPasses:        effect.NPasses,
	}
}

func way(shownScale float64) int {
	if shownScale < 0 {
		return -1
	}
	return 1
}
