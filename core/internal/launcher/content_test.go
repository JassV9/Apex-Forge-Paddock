package launcher

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestReadUIName(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"plain", `{"name": "Ferrari 488 GT3", "brand": "Ferrari"}`, "Ferrari 488 GT3"},
		{"byte order mark", "\xef\xbb\xbf{\"name\": \"Monza\"}", "Monza"},
		{"trailing commas", `{"name": "Spa", "tags": ["a", "b",],}`, "Spa"},
		{"raw line break in a string", "{\"description\": \"one\ntwo\", \"name\": \"Imola\"}", "Imola"},
		{"latin-1 text", "{\"name\": \"N\xfcrburgring\"}", "Nürburgring"},
		{"escaped quote", `{"name": "The \"Ring\""}`, `The "Ring"`},
		{"broken json, name found by hand", `{"name": "Zandvoort", "year": 20 20}`, "Zandvoort"},
		{"no name", `{"brand": "Kunos"}`, ""},
		{"spaces trimmed", `{"name": "  Laguna Seca "}`, "Laguna Seca"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ui.json")
			writeTestFile(t, path, tt.content)
			if got := readUIName(path); got != tt.want {
				t.Errorf("readUIName = %q, want %q", got, tt.want)
			}
		})
	}
	if got := readUIName(filepath.Join(t.TempDir(), "missing.json")); got != "" {
		t.Errorf("missing file gave %q", got)
	}
}

func TestScanContent(t *testing.T) {
	paths := newFixture(t)
	writeTestFile(t, filepath.Join(paths.carDir("abarth500"), "data.acd"), "x")
	writeTestFile(t, filepath.Join(paths.carDir("not_a_car"), "readme.txt"), "x")
	single := paths.trackDir("magione")
	writeTestFile(t, filepath.Join(single, "ui", "ui_track.json"), `{"name": "Magione"}`)
	writeTestFile(t, filepath.Join(single, "ui", "outline.png"), "png")
	orphan := paths.trackDir("fn_cota")
	writeTestFile(t, filepath.Join(orphan, "ui", "club", "ui_track.json"), `{"name": "x - club"}`)

	content, err := ScanContent(paths)
	if err != nil {
		t.Fatal(err)
	}
	wantCars := []contracts.LauncherCar{
		{Id: "abarth500", Name: "abarth500"},
		{Id: "ks_ferrari_488_gt3", Name: "Ferrari 488 GT3"},
	}
	if !reflect.DeepEqual(content.Cars, wantCars) {
		t.Errorf("cars = %+v, want %+v", content.Cars, wantCars)
	}
	wantTracks := []contracts.LauncherTrack{
		{Id: "fn_cota", Name: "Circuit of the Americas", Layouts: []contracts.LauncherLayout{
			{Layout: "gp", Name: "Circuit of the Americas - gp", TestCopy: "gp_paddock"},
			{Layout: "gp_paddock", Name: "Circuit of the Americas - gp_paddock", AiCanDrive: true},
		}},
		{Id: "magione", Name: "Magione", Layouts: []contracts.LauncherLayout{{Name: "Magione"}}},
	}
	if !reflect.DeepEqual(content.Tracks, wantTracks) {
		t.Errorf("tracks = %+v, want %+v", content.Tracks, wantTracks)
	}
}

func TestScanContentWithoutAssettoCorsa(t *testing.T) {
	if _, err := ScanContent(Paths{ACRoot: t.TempDir()}); err == nil {
		t.Fatal("scanning an empty folder should fail")
	}
}
