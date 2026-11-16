package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/launcher"
)

const (
	defaultACRoot   = `C:\Program Files (x86)\Steam\steamapps\common\assettocorsa`
	defaultTemplate = "NJ"
	defaultPort     = 8090
)

// settings are the flags, each with an environment variable and a default.
type settings struct {
	Paths launcher.Paths
	Port  int
}

// parseSettings reads flags over environment variables over defaults.
func parseSettings(args []string, getenv func(string) string) (settings, error) {
	documents := getenv("PADDOCK_DOCUMENTS")
	if documents == "" {
		found, err := launcher.DefaultDocuments()
		if err != nil {
			return settings{}, err
		}
		documents = found
	}
	flags := flag.NewFlagSet("paddock-launcher", flag.ContinueOnError)
	acRoot := flags.String("ac-root", orDefault(getenv("PADDOCK_AC_ROOT"), defaultACRoot),
		"Assetto Corsa install folder (env PADDOCK_AC_ROOT)")
	documentsFlag := flags.String("documents", documents,
		"Documents folder that holds 'Assetto Corsa' (env PADDOCK_DOCUMENTS)")
	template := flags.String("template", orDefault(getenv("PADDOCK_CM_PRESET"), defaultTemplate),
		"Content Manager Quick Drive preset to copy: a name in its Presets\\Quick Drive folder or "+
			"a .cmpreset path (env PADDOCK_CM_PRESET); it is read, never changed")
	port := flags.Int("port", defaultPort, "port on 127.0.0.1")
	if err := flags.Parse(args); err != nil {
		return settings{}, fmt.Errorf("read flags: %w", err)
	}
	return settings{
		Paths: launcher.Paths{
			ACRoot: *acRoot, Documents: *documentsFlag,
			Template: templatePath(*template, getenv("LOCALAPPDATA")),
		},
		Port: *port,
	}, nil
}

// templatePath turns a preset name into its file in Content Manager's Quick Drive folder.
func templatePath(template, localAppData string) string {
	if strings.HasSuffix(strings.ToLower(template), ".cmpreset") || strings.ContainsAny(template, `/\`) {
		return template
	}
	return filepath.Join(localAppData, "AcTools Content Manager", "Presets", "Quick Drive",
		template+".cmpreset")
}

// check fails at start on what would make every launch fail, and warns on the bindings file.
func (s settings) check(logger *slog.Logger) error {
	if _, err := os.Stat(filepath.Join(s.Paths.ACRoot, "content", "cars")); err != nil {
		return fmt.Errorf("no Assetto Corsa at %s (set --ac-root): %w", s.Paths.ACRoot, err)
	}
	if _, err := os.Stat(s.Paths.Template); err != nil {
		return fmt.Errorf("no Quick Drive preset at %s (set --template): %w", s.Paths.Template, err)
	}
	if _, err := os.Stat(s.Paths.Controls()); err != nil {
		logger.Warn("no controls.ini yet: launches are refused until the game has saved one",
			"path", s.Paths.Controls())
	}
	return nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
