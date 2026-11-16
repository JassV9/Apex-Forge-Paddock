package main

import (
	"path/filepath"
	"testing"
)

func TestParseSettings(t *testing.T) {
	local := filepath.Join("C:", "Users", "me", "AppData", "Local")
	presets := filepath.Join(local, "AcTools Content Manager", "Presets", "Quick Drive")
	tests := []struct {
		name         string
		args         []string
		env          map[string]string
		wantRoot     string
		wantTemplate string
		wantPort     int
	}{
		{
			name: "defaults", env: map[string]string{"PADDOCK_DOCUMENTS": "D"},
			wantRoot: defaultACRoot, wantTemplate: filepath.Join(presets, "NJ.cmpreset"), wantPort: 8090,
		},
		{
			name: "environment",
			env: map[string]string{
				"PADDOCK_DOCUMENTS": "D", "PADDOCK_AC_ROOT": "E:/ac", "PADDOCK_CM_PRESET": "NJ26",
			},
			wantRoot: "E:/ac", wantTemplate: filepath.Join(presets, "NJ26.cmpreset"), wantPort: 8090,
		},
		{
			name:     "flags win, a template path is kept",
			args:     []string{"--ac-root", "F:/ac", "--template", "my.cmpreset", "--port", "9000"},
			env:      map[string]string{"PADDOCK_DOCUMENTS": "D", "PADDOCK_AC_ROOT": "E:/ac"},
			wantRoot: "F:/ac", wantTemplate: "my.cmpreset", wantPort: 9000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.env["LOCALAPPDATA"] = local
			got, err := parseSettings(tt.args, func(key string) string { return tt.env[key] })
			if err != nil {
				t.Fatal(err)
			}
			if got.Paths.ACRoot != tt.wantRoot || got.Paths.Template != tt.wantTemplate ||
				got.Port != tt.wantPort || got.Paths.Documents != "D" {
				t.Errorf("settings = %+v", got)
			}
		})
	}
}

func TestParseSettingsRefusesUnknownFlags(t *testing.T) {
	env := func(key string) string { return map[string]string{"PADDOCK_DOCUMENTS": "D"}[key] }
	if _, err := parseSettings([]string{"--skin", "red"}, env); err == nil {
		t.Fatal("an unknown flag should be refused")
	}
}
