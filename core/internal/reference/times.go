package reference

import (
	"math"
	"sort"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Times returns the AI's median, spread and margin per corner, for every corner with at
// least MinAIPasses passes in one AI session (the newest such), by corner number. The passes
// must all be on one setup.
func Times(ai []Pass, names map[int]string) []contracts.AiCornerTime {
	grouped := byCorner(ai)
	numbers := make([]int, 0, len(grouped))
	for number, passes := range grouped {
		grouped[number] = newestSession(passes)
		if len(grouped[number]) >= MinAIPasses {
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)
	corners := make([]contracts.AiCornerTime, 0, len(numbers))
	for _, number := range numbers {
		times := timesOf(grouped[number])
		spread := robustSpread(times)
		corners = append(corners, contracts.AiCornerTime{
			Corner: number, Name: names[number], N: len(times), MedianS: median(times),
			SpreadS: spread, MarginS: math.Max(spread, MarginFloorS),
		})
	}
	return corners
}
