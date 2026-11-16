//go:build !windows

package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// errNotWindows: Assetto Corsa and Content Manager run on Windows only. The package still
// builds elsewhere so its logic is tested in CI on Linux.
var errNotWindows = errors.New("paddock-launcher starts the game on Windows only")

// SystemProcesses would read the process list; it is not implemented off Windows.
type SystemProcesses struct{}

// Running always fails off Windows.
func (SystemProcesses) Running(string) (bool, error) { return false, errNotWindows }

// SystemOpener would hand a URL to Content Manager; it is not implemented off Windows.
type SystemOpener struct{}

// Open always fails off Windows.
func (SystemOpener) Open(string) error { return errNotWindows }

// DefaultDocuments is ~/Documents off Windows.
func DefaultDocuments() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home folder: %w", err)
	}
	return filepath.Join(home, "Documents"), nil
}
