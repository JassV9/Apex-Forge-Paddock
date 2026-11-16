package launcher

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBindingChanges(t *testing.T) {
	tests := []struct {
		name  string
		after string
		want  []string
	}{
		{"same file", controlsText, []string{}},
		{"line endings and spacing only", strings.ReplaceAll(controlsText, "\n", "\r\n"), []string{}},
		{"force feedback level is not a binding", controlsText + "\n[FF_TWEAKS]\nGAIN=80\n", []string{}},
		{"device index moved", strings.Replace(controlsText, "[STEER]\nJOY=0", "[STEER]\nJOY=2", 1), []string{"STEER/JOY"}},
		{"controller renamed", strings.Replace(controlsText, "CON1=Logitech G29", "CON1=vJoy", 1), []string{"CONTROLLERS/CON1"}},
		{"controller removed", strings.Replace(controlsText, "PGUID0=0005346E\n", "", 1), []string{"CONTROLLERS/PGUID0"}},
		{"controller added", strings.Replace(controlsText, "CON1=Logitech G29", "CON1=Logitech G29\nCON2=Arduino", 1), []string{"CONTROLLERS/CON2"}},
		{"modifier index moved", strings.Replace(controlsText, "JOY_MODIFICATOR=-1", "JOY_MODIFICATOR=0", 1), []string{"GEARUP/JOY_MODIFICATOR"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BindingChanges([]byte(controlsText), []byte(tt.after))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BindingChanges = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackupControlsKeepsTheNewestTen(t *testing.T) {
	paths := newFixture(t)
	var last ControlsBackup
	for i := range 12 {
		backup, err := BackupControls(paths, fixedNow.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		last = backup
	}
	kept, err := filepath.Glob(filepath.Join(paths.Backups(), backupPrefix+"*.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != keepBackups {
		t.Errorf("kept %d backups, want %d", len(kept), keepBackups)
	}
	if readTestFile(t, last.Path) != controlsText || last.SHA256 != hashOf([]byte(controlsText)) {
		t.Error("the newest backup is not an exact copy with its hash")
	}
	if strings.Contains(strings.Join(kept, " "), "20261116-183000.000") {
		t.Error("the oldest backup was not removed")
	}
}

func TestBackupControlsWithoutBindings(t *testing.T) {
	paths := newFixture(t)
	if err := os.Remove(paths.Controls()); err != nil {
		t.Fatal(err)
	}
	if _, err := BackupControls(paths, fixedNow); err == nil {
		t.Fatal("a launch without controls.ini to guard should be refused")
	}
}

func TestRestoreControls(t *testing.T) {
	paths := newFixture(t)
	backup, err := BackupControls(paths, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(controlsText, "JOY=0", "JOY=3", 1)
	writeTestFile(t, paths.Controls(), changed)

	if err := RestoreControls(paths, backup, []byte(changed), fixedNow); err != nil {
		t.Fatal(err)
	}
	if readTestFile(t, paths.Controls()) != controlsText {
		t.Error("controls.ini is not the backup again")
	}
	evidence := filepath.Join(paths.Backups(), changedPrefix+"20261116-183000.000.ini")
	if readTestFile(t, evidence) != changed {
		t.Error("the changed file was not kept")
	}
	cfg, err := os.ReadDir(filepath.Dir(paths.Controls()))
	if err != nil || len(cfg) != 1 {
		t.Errorf("cfg/ holds %d files, want only controls.ini (%v)", len(cfg), err)
	}
}

func TestRestoreControlsRefusesATamperedBackup(t *testing.T) {
	paths := newFixture(t)
	backup, err := BackupControls(paths, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, backup.Path, "[CONTROLLERS]\n")
	writeTestFile(t, paths.Controls(), "changed")
	if err := RestoreControls(paths, backup, []byte("changed"), fixedNow); err == nil {
		t.Fatal("a backup that lost its hash must not be written back")
	}
	if readTestFile(t, paths.Controls()) != "changed" {
		t.Error("controls.ini was touched")
	}
}
