package setups

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrFileExists is returned instead of overwriting a setup file.
var ErrFileExists = errors.New("setup file already exists")

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// FileName is a safe file name for a setup, without directories.
func FileName(stem string) string {
	clean := strings.Trim(unsafeFileChars.ReplaceAllString(stem, "_"), "._")
	if clean == "" {
		clean = "setup"
	}
	return clean + ".ini"
}

// Render is the content of an Assetto Corsa setup file: one [ITEM] section with VALUE per
// item, then the car. Items are sorted by name so the same setup renders identically.
func Render(carID string, values Values, description string) string {
	var text strings.Builder
	text.WriteString("[ABOUT]\nAUTHOR=Apex Forge Paddock\nDESCRIPTION=")
	text.WriteString(strings.NewReplacer("\r", " ", "\n", " ").Replace(description))
	text.WriteString("\n\n")
	for _, name := range sortedNames(values) {
		fmt.Fprintf(&text, "[%s]\nVALUE=%s\n\n", name, strconv.FormatFloat(values[name], 'f', -1, 64))
	}
	fmt.Fprintf(&text, "[CAR]\nMODEL=%s\n", carID)
	return text.String()
}

// WriteIni writes a setup file into dir and returns its path. It never overwrites: if the
// file exists, it returns ErrFileExists and leaves it alone.
func WriteIni(dir, stem, carID string, values Values, description string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create setup directory: %w", err)
	}
	path := filepath.Join(dir, FileName(stem))
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640) //nolint:gosec // path is built from a sanitised name
	if errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("%s: %w", path, ErrFileExists)
	}
	if err != nil {
		return "", fmt.Errorf("create setup file: %w", err)
	}
	if _, err := file.WriteString(Render(carID, values, description)); err != nil {
		return "", errors.Join(fmt.Errorf("write setup file: %w", err), file.Close())
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close setup file: %w", err)
	}
	return path, nil
}
