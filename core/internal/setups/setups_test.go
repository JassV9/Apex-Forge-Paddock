package setups

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A few items of the Mazda MX-5 Cup's real setup menu (agent/tests/fixtures).
func mx5() Catalog {
	return Catalog{
		"ARB_FRONT":     {Name: "ARB_FRONT", Label: "ARB Front", Min: 0, Max: 4, Step: 1},
		"CAMBER_LF":     {Name: "CAMBER_LF", Label: "Camber LF", Min: -38, Max: 8, Step: 1},
		"PRESSURE_LF":   {Name: "PRESSURE_LF", Label: "Pressure LF", Min: 15, Max: 40, Step: 1},
		"FRONT_BIAS":    {Name: "FRONT_BIAS", Label: "Brake bias", Min: 50, Max: 80, Step: 2},
		"MGUK_DELIVERY": {Name: "MGUK_DELIVERY", Label: "MGU-K delivery", Min: 0, Max: 5, Step: 1, Excluded: true},
	}
}

func TestValidate(t *testing.T) {
	anchor := Values{"FRONT_BIAS": 55} // loaded from a setup file: off the grid that starts at 50
	tests := []struct {
		name    string
		values  Values
		wantErr error
	}{
		{name: "legal values", values: Values{"ARB_FRONT": 2, "CAMBER_LF": -30}},
		{name: "range ends are legal", values: Values{"ARB_FRONT": 0, "PRESSURE_LF": 40}},
		{name: "on the grid from the minimum", values: Values{"FRONT_BIAS": 58}},
		{name: "on the grid through the current value", values: Values{"FRONT_BIAS": 57}},
		{name: "unknown item", values: Values{"WING_9": 1}, wantErr: ErrUnknownItem},
		{name: "excluded item", values: Values{"MGUK_DELIVERY": 1}, wantErr: ErrExcludedItem},
		{name: "above the range", values: Values{"ARB_FRONT": 5}, wantErr: ErrOutOfRange},
		{name: "below the range", values: Values{"CAMBER_LF": -39}, wantErr: ErrOutOfRange},
		{name: "between two clicks", values: Values{"ARB_FRONT": 2.5}, wantErr: ErrOffGrid},
		{name: "on neither grid", values: Values{"FRONT_BIAS": 57.5}, wantErr: ErrOffGrid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(mx5(), tt.values, anchor)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWithoutAnAnchorUsesOnlyTheMinimumGrid(t *testing.T) {
	if err := Validate(mx5(), Values{"FRONT_BIAS": 57}, nil); !errors.Is(err, ErrOffGrid) {
		t.Errorf("error = %v, want %v", err, ErrOffGrid)
	}
}

func TestStep(t *testing.T) {
	base := Values{"ARB_FRONT": 2, "CAMBER_LF": -30, "FRONT_BIAS": 55}
	tests := []struct {
		name    string
		item    string
		clicks  int
		want    float64
		wantErr error
	}{
		{name: "one click up", item: "ARB_FRONT", clicks: 1, want: 3},
		{name: "two clicks down", item: "ARB_FRONT", clicks: -2, want: 0},
		{name: "a click is one step, from the current value", item: "FRONT_BIAS", clicks: 1, want: 57},
		{name: "past the maximum", item: "ARB_FRONT", clicks: 3, wantErr: ErrOutOfRange},
		{name: "unknown item", item: "WING_9", clicks: 1, wantErr: ErrUnknownItem},
		{name: "item the baseline has no value for", item: "PRESSURE_LF", clicks: 1, wantErr: ErrUnknownItem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variant, err := Step(mx5(), base, tt.item, tt.clicks)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			changes := Diff(base, variant)
			want := []Change{{Name: tt.item, From: base[tt.item], To: tt.want}}
			if !reflect.DeepEqual(changes, want) {
				t.Errorf("changes = %+v, want exactly %+v", changes, want)
			}
		})
	}
	if base["ARB_FRONT"] != 2 {
		t.Error("Step modified the baseline")
	}
	if _, err := Step(mx5(), base, "ARB_FRONT", 0); err == nil {
		t.Error("a change of 0 clicks was accepted")
	}
}

func TestHashDependsOnValuesOnly(t *testing.T) {
	first := Hash(Values{"ARB_FRONT": 2, "CAMBER_LF": -30})
	sameOtherOrder := Hash(Values{"CAMBER_LF": -30, "ARB_FRONT": 2})
	different := Hash(Values{"ARB_FRONT": 3, "CAMBER_LF": -30})
	if first != sameOtherOrder {
		t.Errorf("the same values hash differently: %s and %s", first, sameOtherOrder)
	}
	if first == different || len(first) != 16 {
		t.Errorf("hash %q (other setup: %q)", first, different)
	}
}

func TestWriteIniNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	values := Values{"CAMBER_LF": -30, "ARB_FRONT": 2}

	path, err := WriteIni(dir, "paddock e7 Setup K", "ks_mazda_mx5_cup", values, "Experiment 7\nvariant")
	if err != nil {
		t.Fatalf("WriteIni: %v", err)
	}
	content, err := os.ReadFile(path) //nolint:gosec // file the test just wrote
	if err != nil {
		t.Fatal(err)
	}
	want := "[ABOUT]\nAUTHOR=Apex Forge Paddock\nDESCRIPTION=Experiment 7 variant\n\n" +
		"[ARB_FRONT]\nVALUE=2\n\n[CAMBER_LF]\nVALUE=-30\n\n[CAR]\nMODEL=ks_mazda_mx5_cup\n"
	if string(content) != want {
		t.Errorf("file content:\n%s\nwant:\n%s", content, want)
	}
	if filepath.Base(path) != "paddock_e7_Setup_K.ini" {
		t.Errorf("file name = %s", filepath.Base(path))
	}

	_, err = WriteIni(dir, "paddock e7 Setup K", "ks_mazda_mx5_cup", Values{"ARB_FRONT": 4}, "")
	if !errors.Is(err, ErrFileExists) {
		t.Fatalf("second write: error = %v, want %v", err, ErrFileExists)
	}
	after, _ := os.ReadFile(path) //nolint:gosec // file the test just wrote
	if string(after) != want {
		t.Error("the existing setup file was changed")
	}
}

func TestFileNameIsSafe(t *testing.T) {
	for stem, want := range map[string]string{
		"base_v2":          "base_v2.ini",
		`..\..\evil/name`:  "evil_name.ini",
		"   ":              "setup.ini",
		"Setup K (blind)!": "Setup_K_blind.ini",
	} {
		if got := FileName(stem); got != want || strings.ContainsAny(got, `/\`) {
			t.Errorf("FileName(%q) = %q, want %q", stem, got, want)
		}
	}
}
