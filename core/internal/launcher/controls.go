package launcher

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// keepBackups is how many controls.ini copies of each kind stay in controls_backup.
	keepBackups   = 10
	backupPrefix  = "before_"
	changedPrefix = "changed_"
	stampLayout   = "20060102-150405.000"
)

// ControlsBackup is the copy of controls.ini made before a launch.
type ControlsBackup struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// BackupControls copies controls.ini into the backup folder and keeps the newest ten copies.
// A missing controls.ini refuses the launch: the bindings could not be guarded.
func BackupControls(paths Paths, now time.Time) (ControlsBackup, error) {
	content, err := os.ReadFile(paths.Controls())
	if err != nil {
		return ControlsBackup{}, fmt.Errorf("read the wheel bindings %s: %w", paths.Controls(), err)
	}
	sum := hashOf(content)
	name := backupPrefix + now.UTC().Format(stampLayout) + "_" + sum[:8] + ".ini"
	target := filepath.Join(paths.Backups(), name)
	if err := writeFile(target, content); err != nil {
		return ControlsBackup{}, err
	}
	if err := prune(paths.Backups(), backupPrefix); err != nil {
		return ControlsBackup{}, err
	}
	return ControlsBackup{Path: target, SHA256: sum}, nil
}

// BindingChanges lists what differs between two controls.ini texts in what Content Manager
// rewrites when it saves with a device missing: every [CONTROLLERS] line and every JOY and
// JOY_MODIFICATOR device index. Empty means the bindings are the same.
func BindingChanges(before, after []byte) []string {
	old, current := bindingLines(before), bindingLines(after)
	changed := []string{}
	for key, value := range old {
		if other, ok := current[key]; !ok || other != value {
			changed = append(changed, key)
		}
	}
	for key := range current {
		if _, ok := old[key]; !ok {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	return changed
}

// bindingLines maps "SECTION/KEY" to the value for the lines BindingChanges compares.
func bindingLines(content []byte) map[string]string {
	lines := map[string]string{}
	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToUpper(strings.Trim(line, "[]"))
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key = strings.ToUpper(strings.TrimSpace(key))
		if !found || (section != "CONTROLLERS" && key != "JOY" && key != "JOY_MODIFICATOR") {
			continue
		}
		lines[section+"/"+key] = strings.TrimSpace(value)
	}
	return lines
}

// RestoreControls keeps the changed controls.ini as evidence, then puts the backup back in one
// rename and reads it back. It writes nothing in cfg/ but controls.ini itself.
func RestoreControls(paths Paths, backup ControlsBackup, changed []byte, now time.Time) error {
	content, err := os.ReadFile(backup.Path)
	if err != nil {
		return fmt.Errorf("read the backup %s: %w", backup.Path, err)
	}
	if hashOf(content) != backup.SHA256 {
		return fmt.Errorf("the backup %s no longer has the hash it was made with", backup.Path)
	}
	evidence := filepath.Join(paths.Backups(), changedPrefix+now.UTC().Format(stampLayout)+".ini")
	if err := writeFile(evidence, changed); err != nil {
		return err
	}
	if err := prune(paths.Backups(), changedPrefix); err != nil {
		return err
	}
	staged := filepath.Join(paths.Backups(), "restore.tmp")
	if err := writeFile(staged, content); err != nil {
		return err
	}
	if err := os.Rename(staged, paths.Controls()); err != nil {
		return fmt.Errorf("put the backup back as %s: %w", paths.Controls(), err)
	}
	written, err := os.ReadFile(paths.Controls())
	if err != nil {
		return fmt.Errorf("read back %s: %w", paths.Controls(), err)
	}
	if hashOf(written) != backup.SHA256 {
		return fmt.Errorf("read-back of %s does not match the backup", paths.Controls())
	}
	return nil
}

// prune deletes all but the newest keepBackups files with this prefix (names sort by time).
func prune(dir, prefix string) error {
	matches, err := filepath.Glob(filepath.Join(dir, prefix+"*.ini"))
	if err != nil {
		return fmt.Errorf("list backups in %s: %w", dir, err)
	}
	sort.Strings(matches)
	for len(matches) > keepBackups {
		if err := os.Remove(matches[0]); err != nil {
			return fmt.Errorf("remove old backup: %w", err)
		}
		matches = matches[1:]
	}
	return nil
}

func hashOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// writeFile writes through a temporary file and a rename, creating the folder if needed.
func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", temporary, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("rename %s: %w", temporary, err)
	}
	return nil
}
