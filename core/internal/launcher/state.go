package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// state is what the launcher remembers in launch/state.json, so a launcher closed while the
// game runs still checks the bindings when it is started again.
type state struct {
	LastLaunch *contracts.LaunchRecord  `json:"last_launch,omitempty"`
	Controls   *contracts.ControlsCheck `json:"controls,omitempty"`
	Backup     *ControlsBackup          `json:"backup,omitempty"`
}

// watching is true while a launch's bindings have not been checked yet.
func (s state) watching() bool {
	return s.Controls != nil && s.Controls.State == contracts.ControlsCheckStateWatching &&
		s.Backup != nil
}

func loadState(path string) (state, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("read launcher state %s: %w", path, err)
	}
	var loaded state
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return state{}, fmt.Errorf("parse launcher state %s: %w", path, err)
	}
	return loaded, nil
}

func saveState(path string, current state) error {
	raw, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("encode launcher state: %w", err)
	}
	return writeFile(path, raw)
}
