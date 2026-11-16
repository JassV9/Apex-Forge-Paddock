package passes

import (
	"math"
	"slices"
	"sort"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Summarize returns one summary per corner that has a pass, by corner number: the count of
// passes and of valid ones, and over the valid passes the shortest zone time (with its pass),
// the median and the sample standard deviation. Invalid passes never count toward a time.
func Summarize(records []contracts.PassRecord, names map[int]string) []contracts.CornerSummary {
	byCorner := map[int][]contracts.PassRecord{}
	for _, record := range records {
		byCorner[record.Corner] = append(byCorner[record.Corner], record)
	}
	numbers := make([]int, 0, len(byCorner))
	for number := range byCorner {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	summaries := make([]contracts.CornerSummary, 0, len(numbers))
	for _, number := range numbers {
		summaries = append(summaries, summarizeCorner(number, names[number], byCorner[number]))
	}
	return summaries
}

func summarizeCorner(number int, name string, records []contracts.PassRecord) contracts.CornerSummary {
	summary := contracts.CornerSummary{Corner: number, Name: name, Passes: len(records)}
	var times []float64
	for _, record := range records {
		if !record.Valid {
			continue
		}
		times = append(times, record.DurationS)
		if summary.BestS == nil || record.DurationS < *summary.BestS {
			best, seq := record.DurationS, record.Seq
			summary.BestS, summary.BestSeq = &best, &seq
		}
	}
	summary.ValidPasses = len(times)
	if len(times) == 0 {
		return summary
	}
	median := medianOf(times)
	summary.MedianS = &median
	if len(times) > 1 {
		spread := sampleStdDev(times)
		summary.SpreadS = &spread
	}
	return summary
}

func medianOf(values []float64) float64 {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// sampleStdDev is the standard deviation with n-1 in the denominator; it needs two values.
func sampleStdDev(values []float64) float64 {
	mean := 0.0
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	squares := 0.0
	for _, value := range values {
		squares += (value - mean) * (value - mean)
	}
	return math.Sqrt(squares / float64(len(values)-1))
}
