package launcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// PracticeMode is Content Manager's Quick Drive page for a practice session.
const PracticeMode = "/Pages/Drive/QuickDrive_Practice.xaml"

// defaultPracticeData is the ModeData of a practice from the pits, used when the template is
// another mode (its ModeData would not fit Practice).
var defaultPracticeData = map[string]any{
	"StartType": "PIT", "Penalties": false, "PlayerBallast": 0, "PlayerRestrictor": 0,
}

// TrackID is how Content Manager names a layout: "track/layout", or "track" for the main one.
func TrackID(trackID, layout string) string {
	if layout == "" {
		return trackID
	}
	return trackID + "/" + layout
}

// DefaultWeather is the weather every launch drives in unless told otherwise. It is fixed, not
// the template's: a preset without one lets Content Manager pick (seen in game: light drizzle
// and 93 % grip), and AI calibration, experiments and hotlaps only compare in like conditions.
const DefaultWeather = "3_clear"

// optimumTrack is Content Manager's "Optimum" track state: full grip from the start, no
// transfer, no randomness. A track state "set by weather" gave 93 % grip in clear weather; the
// AI was calibrated at 100 %.
const optimumTrack = `{"s":1.0,"t":1.0,"r":0.0,"g":1,"d":"Perfect track for hotlapping.","w":false}`

// BuildPreset copies a Quick Drive preset and sets the car, the layout, Practice, the pit start
// and, when weather is not empty, fixed conditions: that weather, real conditions off and the
// optimum track. Everything else (time, temperature, assists) stays the owner's.
func BuildPreset(template []byte, request contracts.LaunchRequest, weather string) ([]byte, error) {
	var preset map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimPrefix(template, []byte("\xef\xbb\xbf")), &preset); err != nil {
		return nil, fmt.Errorf("read the Quick Drive template: %w", err)
	}
	modeData, err := practiceData(preset)
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"CarId":    request.CarId,
		"TrackId":  TrackID(request.TrackId, request.Layout),
		"Mode":     PracticeMode,
		"ModeData": modeData,
	}
	if weather != "" {
		values["WeatherId"], values["RealConditions"] = weather, false
		values["TrackPropertiesData"] = optimumTrack
		delete(preset, "TrackPropertiesPresetFilename") // a preset file would win over the data
	}
	for key, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode preset field %s: %w", key, err)
		}
		preset[key] = encoded
	}
	return json.Marshal(preset)
}

// practiceData is the template's ModeData (a JSON object inside a string) with StartType PIT.
func practiceData(preset map[string]json.RawMessage) (string, error) {
	data := map[string]any{}
	var mode, encoded string
	_ = json.Unmarshal(preset["Mode"], &mode)
	if mode == PracticeMode && json.Unmarshal(preset["ModeData"], &encoded) == nil &&
		json.Unmarshal([]byte(encoded), &data) == nil {
		data["StartType"] = "PIT"
	} else {
		for key, value := range defaultPracticeData {
			data[key] = value
		}
	}
	out, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("encode practice mode data: %w", err)
	}
	return string(out), nil
}

// QuickDriveURL asks Content Manager to run Quick Drive with a preset file and press Go.
// %20 rather than + for spaces, which every URL decoder reads the same way.
func QuickDriveURL(presetPath string) string {
	escaped := strings.ReplaceAll(url.QueryEscape(presetPath), "+", "%20")
	return "acmanager://race/quick?presetFile=" + escaped
}
