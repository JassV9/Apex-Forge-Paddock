package sensitivity

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// statement is the base setup's hypothesis in words.
func statement(changes int, carName string) string {
	noun := "changes"
	if changes == 1 {
		noun = "change"
	}
	return fmt.Sprintf("Base setup: %d measured %s on %s", changes, noun, carName)
}

func clicksOf(chosen contracts.BaseSetup) map[string]int {
	clicks := make(map[string]int, len(chosen.Changes))
	for _, change := range chosen.Changes {
		clicks[change.Parameter] = change.Clicks
	}
	return clicks
}

// evidenceOf says what each change rests on, as the hypothesis keeps it.
func evidenceOf(chosen contracts.BaseSetup) []string {
	evidence := make([]string, 0, len(chosen.Changes))
	for _, change := range chosen.Changes {
		ids := make([]string, len(change.ExperimentIds))
		for i, id := range change.ExperimentIds {
			ids[i] = strconv.Itoa(id)
		}
		evidence = append(evidence, fmt.Sprintf("%s %+d: %+.4f [%+.4f, %+.4f] on %s, experiments %s",
			change.Parameter, change.Clicks, change.Estimate, change.CiLow, change.CiHigh,
			chosen.Channel, strings.Join(ids, ", ")))
	}
	return evidence
}

// predictionSpec is the engine's prediction as the experiment stores it: a direction and the
// size of the change, written down before a lap is driven.
func predictionSpec(prediction contracts.BaseSetupPrediction) contracts.PredictionSpec {
	if prediction.Estimate < 0 {
		return contracts.PredictionSpec{
			Channel: prediction.Channel, Direction: contracts.PredictionSpecDirectionDecrease,
			Low: max(-prediction.CiHigh, 0), High: max(-prediction.CiLow, 0),
		}
	}
	return contracts.PredictionSpec{
		Channel: prediction.Channel, Direction: contracts.PredictionSpecDirectionIncrease,
		Low: max(prediction.CiLow, 0), High: max(prediction.CiHigh, 0),
	}
}

// nothingQualifies says why there is no base setup, and what to do about it.
func nothingQualifies(chosen contracts.BaseSetup) string {
	const advice = "Train the AI on this car (free laps), then let it drive one-item experiments; " +
		"a change goes into a base setup once its interval on %s lies wholly on the better side of zero " +
		"and it holds up against every other measured change (Benjamini-Hochberg)."
	if len(chosen.Measured) == 0 {
		return "This car has measured nothing yet. " + fmt.Sprintf(advice, chosen.Channel)
	}
	measured := make([]string, len(chosen.Measured))
	for i, row := range chosen.Measured {
		measured[i] = fmt.Sprintf("%s %+d %s (%d experiments)", row.Label, row.Clicks, row.KnowledgeLevel, row.Experiments)
	}
	return fmt.Sprintf("No measured change qualifies. Measured on this car: %s. ", strings.Join(measured, "; ")) +
		fmt.Sprintf(advice, chosen.Channel)
}
