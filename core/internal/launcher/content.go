package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// TestCopySuffix marks the copy of a layout made by tools/prepare_test_layout.py, the only
// kind of layout on which the agent may hand the car to AC's AI (docs/adr/0014).
const TestCopySuffix = "_paddock"

// ScanContent lists the installed cars and tracks. Unreadable ui files fall back to ids.
func ScanContent(paths Paths) (contracts.LauncherContent, error) {
	cars, err := scanCars(filepath.Join(paths.ACRoot, "content", "cars"))
	if err != nil {
		return contracts.LauncherContent{}, err
	}
	tracks, err := scanTracks(paths)
	if err != nil {
		return contracts.LauncherContent{}, err
	}
	return contracts.LauncherContent{AcRoot: paths.ACRoot, Cars: cars, Tracks: tracks}, nil
}

func scanCars(dir string) ([]contracts.LauncherCar, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read installed cars in %s: %w", dir, err)
	}
	cars := []contracts.LauncherCar{}
	for _, entry := range entries {
		if !entry.IsDir() || !isCar(filepath.Join(dir, entry.Name())) {
			continue
		}
		name := readUIName(filepath.Join(dir, entry.Name(), "ui", "ui_car.json"))
		cars = append(cars, contracts.LauncherCar{Id: entry.Name(), Name: orID(name, entry.Name())})
	}
	sort.Slice(cars, func(i, j int) bool { return lessFold(cars[i].Name, cars[j].Name) })
	return cars, nil
}

// isCar is true for a folder with packed or unpacked car data, or at least a ui file.
func isCar(dir string) bool {
	for _, marker := range []string{"data.acd", "data", filepath.Join("ui", "ui_car.json")} {
		if exists(filepath.Join(dir, marker)) {
			return true
		}
	}
	return false
}

func scanTracks(paths Paths) ([]contracts.LauncherTrack, error) {
	dir := filepath.Join(paths.ACRoot, "content", "tracks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read installed tracks in %s: %w", dir, err)
	}
	tracks := []contracts.LauncherTrack{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if track, ok := scanTrack(paths.trackDir(entry.Name()), entry.Name()); ok {
			tracks = append(tracks, track)
		}
	}
	sort.Slice(tracks, func(i, j int) bool { return lessFold(tracks[i].Name, tracks[j].Name) })
	return tracks, nil
}

// scanTrack reads the layouts the way Content Manager lists them: ui/ui_track.json is the
// main layout (id ""), ui/<layout>/ui_track.json with a <layout> folder is another one.
func scanTrack(dir, id string) (contracts.LauncherTrack, bool) {
	layouts := []contracts.LauncherLayout{}
	if exists(filepath.Join(dir, "ui", "ui_track.json")) {
		name := readUIName(filepath.Join(dir, "ui", "ui_track.json"))
		layouts = append(layouts, contracts.LauncherLayout{Name: orID(name, id)})
	}
	entries, err := os.ReadDir(filepath.Join(dir, "ui"))
	if err != nil {
		return contracts.LauncherTrack{}, false
	}
	for _, entry := range entries {
		if layoutInstalled(dir, entry.Name()) {
			name := readUIName(filepath.Join(dir, "ui", entry.Name(), "ui_track.json"))
			layouts = append(layouts, contracts.LauncherLayout{
				Layout: entry.Name(), Name: orID(name, entry.Name()),
			})
		}
	}
	if len(layouts) == 0 {
		return contracts.LauncherTrack{}, false
	}
	markTestCopies(layouts)
	return contracts.LauncherTrack{Id: id, Name: trackName(layouts[0].Name), Layouts: layouts}, true
}

// layoutInstalled is true for a named layout with both its ui file and its data folder.
func layoutInstalled(trackDir, layout string) bool {
	if layout == "" {
		return exists(filepath.Join(trackDir, "ui", "ui_track.json"))
	}
	return exists(filepath.Join(trackDir, "ui", layout, "ui_track.json")) &&
		isDir(filepath.Join(trackDir, layout))
}

func markTestCopies(layouts []contracts.LauncherLayout) {
	present := map[string]bool{}
	for _, layout := range layouts {
		present[layout.Layout] = true
	}
	for i := range layouts {
		layouts[i].AiCanDrive = isTestCopy(layouts[i].Layout)
		copyID := layouts[i].Layout + TestCopySuffix
		if layouts[i].Layout != "" && !layouts[i].AiCanDrive && present[copyID] {
			layouts[i].TestCopy = copyID
		}
	}
}

func isTestCopy(layout string) bool {
	return strings.HasSuffix(layout, TestCopySuffix) && layout != TestCopySuffix
}

// trackName drops the layout part of a name such as "Circuit of the Americas - GP".
func trackName(layoutName string) string {
	if head, _, found := strings.Cut(layoutName, " - "); found && head != "" {
		return head
	}
	return layoutName
}

func orID(name, id string) string {
	if name == "" {
		return id
	}
	return name
}

func lessFold(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) }

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
