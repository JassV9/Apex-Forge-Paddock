package reference

import (
	"fmt"
	"math"
	"sort"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

const (
	// MinAIPasses is how many AI passes of a corner on one setup a comparison needs: with
	// fewer there is no spread to judge a gap against.
	MinAIPasses = 3
	// MarginFloorS is the smallest margin a corner is classed with, seconds. The AI repeats a
	// corner within a few milliseconds; a gap that small is not a technique to practise.
	MarginFloorS = 0.01
)

// Pass is one valid corner pass as the comparison reads it.
type Pass struct {
	SessionID int64
	Seq       int
	Corner    int
	DurationS float64
	SetupHash string
	Metrics   map[string]float64
}

// Input is what one session's comparison is made from.
type Input struct {
	// Human are the session's valid passes on the compared setup.
	Human []Pass
	// AI are the valid AI passes on the same car and track, on every setup.
	AI []Pass
	// SetupHash is the compared setup.
	SetupHash string
	// Values are the setups' items by agent hash, for finding the nearest setup.
	Values map[string]setups.Values
	Names  map[int]string
}

// aiMatch is the AI passes one corner is compared with.
type aiMatch struct {
	kind    contracts.CornerReferenceMatch
	hash    string
	changes int
	passes  []Pass
}

// Compare returns one comparison per corner the driver has a pass of, by corner number.
func Compare(in Input) []contracts.CornerReference {
	human, ai := byCorner(in.Human), byCorner(in.AI)
	numbers := make([]int, 0, len(human))
	for number := range human {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	corners := make([]contracts.CornerReference, 0, len(numbers))
	for _, number := range numbers {
		match := matchAI(in, bySetup(ai[number]))
		corners = append(corners, compareCorner(number, in.Names[number], human[number], match))
	}
	return corners
}

func byCorner(passes []Pass) map[int][]Pass {
	grouped := map[int][]Pass{}
	for _, pass := range passes {
		grouped[pass.Corner] = append(grouped[pass.Corner], pass)
	}
	return grouped
}

// bySetup groups the AI passes of one corner by setup, each setup with the passes of one
// AI session only (newestSession).
func bySetup(passes []Pass) map[string][]Pass {
	grouped := map[string][]Pass{}
	for _, pass := range passes {
		grouped[pass.SetupHash] = append(grouped[pass.SetupHash], pass)
	}
	for hash, list := range grouped {
		grouped[hash] = newestSession(list)
	}
	return grouped
}

// newestSession keeps the passes of the newest AI session that has MinAIPasses of them; with
// none, those of the session with the most. The AI's grip calibration (docs/adr/0016) is not
// stored per pass, so passes of two sessions can be two different drivers; the newest
// session is the AI as it drives today.
func newestSession(passes []Pass) []Pass {
	bySession := map[int64][]Pass{}
	for _, pass := range passes {
		bySession[pass.SessionID] = append(bySession[pass.SessionID], pass)
	}
	var newest, most int64 = -1, -1
	for id, list := range bySession {
		if len(list) >= MinAIPasses && id > newest {
			newest = id
		}
		if most < 0 || len(list) > len(bySession[most]) || (len(list) == len(bySession[most]) && id > most) {
			most = id
		}
	}
	if newest >= 0 {
		return bySession[newest]
	}
	return bySession[most]
}

// matchAI takes the AI passes on the same setup; failing that, those on the setup that
// differs from it in the fewest items (then: more passes, then the lower hash).
func matchAI(in Input, ai map[string][]Pass) aiMatch {
	same := ai[in.SetupHash]
	if in.SetupHash != "" && len(same) >= MinAIPasses {
		return aiMatch{kind: contracts.CornerReferenceMatchSame, hash: in.SetupHash, passes: same}
	}
	mine, known := in.Values[in.SetupHash]
	best := aiMatch{kind: contracts.CornerReferenceMatchNone, passes: same}
	if !known {
		return best
	}
	for hash, passes := range ai {
		theirs, ok := in.Values[hash]
		if !ok || hash == in.SetupHash || len(passes) < MinAIPasses {
			continue
		}
		candidate := aiMatch{kind: contracts.CornerReferenceMatchNearest, hash: hash,
			changes: len(setups.Diff(mine, theirs)), passes: passes}
		if best.kind == contracts.CornerReferenceMatchNone || closer(candidate, best) {
			best = candidate
		}
	}
	return best
}

func closer(a, b aiMatch) bool {
	if a.changes != b.changes {
		return a.changes < b.changes
	}
	if len(a.passes) != len(b.passes) {
		return len(a.passes) > len(b.passes)
	}
	return a.hash < b.hash
}

func compareCorner(number int, name string, human []Pass, match aiMatch) contracts.CornerReference {
	best := fastest(human)
	ref := contracts.CornerReference{
		Corner: number, Name: name, HumanN: len(human), HumanBestS: best.DurationS,
		HumanBestSeq: best.Seq, HumanMedianS: median(timesOf(human)), Match: match.kind,
		AiN: len(match.passes), Differences: []contracts.FeatureDifference{},
	}
	if match.kind == contracts.CornerReferenceMatchNone {
		ref.Class = contracts.CornerReferenceClassNoReference
		ref.Reason = fmt.Sprintf("%d AI passes of this corner on this setup; %d are needed, "+
			"and no other setup the AI drove here has them", len(match.passes), MinAIPasses)
		return ref
	}
	applyAI(&ref, match)
	ref.Differences = differences(human, match.passes)
	return ref
}

// applyAI fills in the AI's numbers, the gaps and the class.
func applyAI(ref *contracts.CornerReference, match aiMatch) {
	times := timesOf(match.passes)
	aiMedian, spread := median(times), robustSpread(times)
	aiBest := fastest(match.passes)
	sessionID := int(aiBest.SessionID)
	margin := math.Max(spread, MarginFloorS)
	deltaBest, deltaMedian := ref.HumanBestS-aiMedian, ref.HumanMedianS-aiMedian
	ref.AiSetupHash, ref.SetupChanges = &match.hash, &match.changes
	ref.AiMedianS, ref.AiSpreadS, ref.AiBestS = &aiMedian, &spread, &aiBest.DurationS
	ref.AiBestSessionId, ref.AiBestSeq = &sessionID, &aiBest.Seq
	ref.DeltaBestS, ref.DeltaMedianS, ref.MarginS = &deltaBest, &deltaMedian, &margin
	ref.Class, ref.Reason = classify(deltaMedian, margin)
	if match.kind == contracts.CornerReferenceMatchNearest {
		ref.Reason += fmt.Sprintf("; the AI drove a different setup here (%d items differ), "+
			"so part of the gap can be the setup", match.changes)
	}
}

// classify is the rule of docs/adr/0019: slower than the AI by more than its own scatter is
// the driver; anything else is what the car and setup allow, as far as the AI shows.
func classify(deltaMedian, margin float64) (contracts.CornerReferenceClass, string) {
	switch {
	case deltaMedian > margin:
		return contracts.CornerReferenceClassDriver, fmt.Sprintf(
			"your typical pass is %.3f s slower than the AI's median, more than its margin of %.3f s",
			deltaMedian, margin)
	case deltaMedian < -margin:
		return contracts.CornerReferenceClassCar, fmt.Sprintf(
			"your typical pass is %.3f s faster than the AI's median; the AI drives below the limit",
			-deltaMedian)
	default:
		return contracts.CornerReferenceClassCar, fmt.Sprintf(
			"your typical pass is within the AI's margin of %.3f s", margin)
	}
}
