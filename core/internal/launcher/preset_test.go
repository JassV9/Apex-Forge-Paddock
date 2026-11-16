package launcher

import (
	"encoding/json"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func decodePreset(t *testing.T, preset []byte) (map[string]any, map[string]any) {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(preset, &fields); err != nil {
		t.Fatal(err)
	}
	var modeData map[string]any
	if err := json.Unmarshal([]byte(fields["ModeData"].(string)), &modeData); err != nil {
		t.Fatal(err)
	}
	return fields, modeData
}

func TestBuildPreset(t *testing.T) {
	otherMode := `{"Mode":"/Pages/Drive/QuickDrive_Race.xaml","ModeData":"{\"AiLevel\":90}",` +
		`"CarId":"x","TrackId":"y","Time":3600}`
	tests := []struct {
		name         string
		template     string
		request      contracts.LaunchRequest
		wantTrack    string
		wantModeData map[string]any
		wantKept     map[string]any
	}{
		{
			name:         "practice template keeps its mode data, starts in the pits",
			template:     templatePreset,
			request:      contracts.LaunchRequest{CarId: "ks_ferrari_488_gt3", TrackId: "fn_cota", Layout: "gp_paddock"},
			wantTrack:    "fn_cota/gp_paddock",
			wantModeData: map[string]any{"StartType": "PIT", "Penalties": false, "PlayerBallast": 0.0},
			wantKept:     map[string]any{"Temperature": 26.0, "Time": 43200.0, "AssistsData": `{"Abs":1,"TractionControl":1}`},
		},
		{
			name:      "another mode gets the default practice data",
			template:  otherMode,
			request:   contracts.LaunchRequest{CarId: "abarth500", TrackId: "magione"},
			wantTrack: "magione",
			wantModeData: map[string]any{
				"StartType": "PIT", "Penalties": false, "PlayerBallast": 0.0, "PlayerRestrictor": 0.0,
			},
			wantKept: map[string]any{"Time": 3600.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, err := BuildPreset([]byte(tt.template), tt.request, "")
			if err != nil {
				t.Fatal(err)
			}
			fields, modeData := decodePreset(t, preset)
			want := map[string]any{"CarId": tt.request.CarId, "TrackId": tt.wantTrack, "Mode": PracticeMode}
			for key, value := range tt.wantKept {
				want[key] = value
			}
			for key, value := range want {
				if fields[key] != value {
					t.Errorf("%s = %v, want %v", key, fields[key], value)
				}
			}
			for key, value := range tt.wantModeData {
				if modeData[key] != value {
					t.Errorf("ModeData.%s = %v, want %v", key, modeData[key], value)
				}
			}
		})
	}
}

func TestBuildPresetFixesTheWeather(t *testing.T) {
	tests := []struct {
		name, template, weather string
		want                    any
	}{
		{"a template without weather gets the fixed one", templatePreset, DefaultWeather, DefaultWeather},
		{"a template's own weather and track state are replaced",
			`{"WeatherId":"*15","RealConditions":true,"TrackPropertiesPresetFilename":"Auto.cmpreset"}`,
			"3_clear", "3_clear"},
		{"empty keeps the template's", `{"WeatherId":"*15"}`, "", "*15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, err := BuildPreset([]byte(tt.template), contracts.LaunchRequest{CarId: "a", TrackId: "b"}, tt.weather)
			if err != nil {
				t.Fatal(err)
			}
			fields, _ := decodePreset(t, preset)
			if fields["WeatherId"] != tt.want {
				t.Errorf("WeatherId = %v, want %v", fields["WeatherId"], tt.want)
			}
			if tt.weather != "" && fields["RealConditions"] != false {
				t.Errorf("RealConditions = %v: real weather would override the fixed one", fields["RealConditions"])
			}
			_, hasTrackPreset := fields["TrackPropertiesPresetFilename"]
			if tt.weather != "" && (fields["TrackPropertiesData"] != optimumTrack || hasTrackPreset) {
				t.Errorf("track state %v (preset file kept: %v), want the optimum track",
					fields["TrackPropertiesData"], hasTrackPreset)
			}
		})
	}
}

func TestBuildPresetRefusesBrokenTemplate(t *testing.T) {
	if _, err := BuildPreset([]byte("not json"), contracts.LaunchRequest{CarId: "a", TrackId: "b"}, ""); err == nil {
		t.Fatal("a broken template should be refused")
	}
}

func TestQuickDriveURL(t *testing.T) {
	got := QuickDriveURL(`C:\Users\Jason\Documents\Assetto Corsa\paddock\launch\paddock.cmpreset`)
	want := "acmanager://race/quick?presetFile=C%3A%5CUsers%5CJason%5CDocuments%5CAssetto%20Corsa" +
		"%5Cpaddock%5Claunch%5Cpaddock.cmpreset"
	if got != want {
		t.Errorf("QuickDriveURL = %s, want %s", got, want)
	}
}
