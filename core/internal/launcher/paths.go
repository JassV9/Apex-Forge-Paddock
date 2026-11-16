// Package launcher starts Assetto Corsa through Content Manager's Quick Drive and guards the
// owner's wheel bindings (docs/adr/0017-host-launcher.md). It runs on the Windows host, not in
// Docker; the game, Content Manager and the process list sit behind small interfaces.
package launcher

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrInvalid reports a request that names nothing installed or is malformed (HTTP 422).
var ErrInvalid = errors.New("invalid launch request")

// ErrBusy reports a launch refused because the game runs or a launch is still watched (409).
var ErrBusy = errors.New("launcher is busy")

// Paths are the folders the launcher reads, and the one folder it writes to.
type Paths struct {
	// ACRoot is the Assetto Corsa install folder (holds content/cars and content/tracks).
	ACRoot string
	// Documents is the user's Documents folder (holds "Assetto Corsa/cfg/controls.ini").
	Documents string
	// Template is the Content Manager Quick Drive preset the launch preset is copied from.
	Template string
}

// UserFolder is Documents/Assetto Corsa.
func (p Paths) UserFolder() string { return filepath.Join(p.Documents, "Assetto Corsa") }

// Controls is the game's bindings file, the only file outside Paddock's folder ever written,
// and only to put back its own backup.
func (p Paths) Controls() string { return filepath.Join(p.UserFolder(), "cfg", "controls.ini") }

// Paddock is Documents/Assetto Corsa/paddock, where everything the launcher makes lives.
func (p Paths) Paddock() string { return filepath.Join(p.UserFolder(), "paddock") }

// Backups holds the controls.ini copies made before each launch.
func (p Paths) Backups() string { return filepath.Join(p.Paddock(), "controls_backup") }

// LaunchDir holds the preset handed to Content Manager and the launcher's state.
func (p Paths) LaunchDir() string { return filepath.Join(p.Paddock(), "launch") }

// Preset is the Quick Drive preset written for each launch.
func (p Paths) Preset() string { return filepath.Join(p.LaunchDir(), "paddock.cmpreset") }

// StateFile keeps the last launch and the bindings check across launcher restarts.
func (p Paths) StateFile() string { return filepath.Join(p.LaunchDir(), "state.json") }

func (p Paths) carDir(id string) string { return filepath.Join(p.ACRoot, "content", "cars", id) }

func (p Paths) trackDir(id string) string {
	return filepath.Join(p.ACRoot, "content", "tracks", id)
}

// checkFolderName refuses anything that is not a single plain folder name, so an id from a
// request can never point outside content/.
func checkFolderName(what, name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:*?"<>|`) {
		return fmt.Errorf("%w: %s %q is not a folder name", ErrInvalid, what, name)
	}
	return nil
}
