package passes

import (
	"math"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// keptIndices thins n samples to at most maxPoints: every step-th sample from the first, and
// the last one, so a trace always spans the whole pass. Nothing is averaged or interpolated.
func keptIndices(n, maxPoints int) (indices []int, step int) {
	step = 1
	if n > maxPoints && maxPoints > 1 {
		step = (n - 1 + maxPoints - 2) / (maxPoints - 1) // ceil((n-1)/(maxPoints-1))
	}
	for i := 0; i < n; i += step {
		indices = append(indices, i)
	}
	if last := n - 1; n > 0 && indices[len(indices)-1] != last {
		indices = append(indices, last)
	}
	return indices, step
}

// buildTrace returns the kept samples as columns, with the slip angle of each axle.
func buildTrace(rows []sampleRow, maxPoints int) contracts.PassTrace {
	indices, step := keptIndices(len(rows), maxPoints)
	kept := make([]sampleRow, len(indices))
	for i, index := range indices {
		kept[i] = rows[index]
	}
	return contracts.PassTrace{
		NRecorded: len(rows),
		N:         len(kept),
		Step:      step,
		Columns:   columnsOf(kept),
		Axles:     axlesOf(kept),
	}
}

func column(rows []sampleRow, value func(sampleRow) float64) []float64 {
	out := make([]float64, len(rows))
	for i, row := range rows {
		out[i] = value(row)
	}
	return out
}

// columnsOf turns rows into the 24 channel columns, in the order of sampleRow.channels.
func columnsOf(rows []sampleRow) contracts.TraceColumns {
	var c contracts.TraceColumns
	targets := []*[]float64{
		&c.T, &c.D, &c.V, &c.Gas, &c.Brake, &c.Steer, &c.LatG, &c.LonG, &c.Yaw,
		&c.SaFl, &c.SaFr, &c.SaRl, &c.SaRr, &c.SrFl, &c.SrFr, &c.SrRl, &c.SrRr,
		&c.LoadFl, &c.LoadFr, &c.LoadRl, &c.LoadRr, &c.Out, &c.RhF, &c.RhR,
	}
	for _, target := range targets {
		*target = make([]float64, 0, len(rows))
	}
	for _, row := range rows {
		values := row.channels()
		for i, target := range targets {
			*target = append(*target, values[i])
		}
	}
	return c
}

func axlesOf(rows []sampleRow) contracts.TraceAxles {
	return contracts.TraceAxles{
		FrontSlipDeg: column(rows, func(r sampleRow) float64 { return axleSlip(r.SaFl, r.SaFr, r.LoadFl, r.LoadFr) }),
		RearSlipDeg:  column(rows, func(r sampleRow) float64 { return axleSlip(r.SaRl, r.SaRr, r.LoadRl, r.LoadRr) }),
	}
}

// axleSlip is engine/paddock_engine/features/axles.py's axle slip: the mean of the two
// tyres' absolute slip angles weighted by their load (negative load counts as none), or the
// plain mean when the axle carries 1 N or less.
func axleSlip(angleA, angleB, loadA, loadB float64) float64 {
	angleA, angleB = math.Abs(angleA), math.Abs(angleB)
	loadA, loadB = math.Max(loadA, 0), math.Max(loadB, 0)
	total := loadA + loadB
	if total <= 1 {
		return (angleA + angleB) / 2
	}
	return (angleA*loadA + angleB*loadB) / total
}
