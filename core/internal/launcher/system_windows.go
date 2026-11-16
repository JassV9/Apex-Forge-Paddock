//go:build windows

package launcher

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SystemProcesses reads the Windows process list.
type SystemProcesses struct{}

// Running is true when a process with this executable name exists (case-insensitive).
func (SystemProcesses) Running(image string) (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, fmt.Errorf("snapshot the process list: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), image) {
			return true, nil
		}
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, nil
	}
	return false, fmt.Errorf("walk the process list: %w", err)
}

// SystemOpener hands a URL to the program Windows has registered for its scheme, the same
// way a click on an acmanager:// link does (ShellExecute "open"). No shell parses the URL.
type SystemOpener struct{}

// Open starts the handler and returns once Windows has handed the URL over.
func (SystemOpener) Open(target string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("encode %s: %w", target, err)
	}
	if err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("open %s (is Content Manager installed and registered?): %w", target, err)
	}
	return nil
}

// DefaultDocuments is the user's Documents folder, wherever it was moved (OneDrive included).
func DefaultDocuments() (string, error) {
	path, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		return "", fmt.Errorf("find the Documents folder: %w", err)
	}
	return path, nil
}
