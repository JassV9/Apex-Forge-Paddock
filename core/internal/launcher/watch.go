package launcher

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// maxListedChanges caps how many changed binding lines the status names.
const maxListedChanges = 8

// watch waits for the game to start (when asked to) and to exit, then checks the bindings.
// When the launcher is stopped first, the state still says watching and the next start
// resumes here.
func (s *Service) watch(backup ControlsBackup, waitForStart bool) {
	defer s.watches.Done()
	if waitForStart && !s.waitFor(true, s.cfg.Timing.StartTimeout) {
		if s.ctx.Err() != nil {
			return
		}
		s.cfg.Logger.Warn("the game did not start; checking the bindings anyway",
			"timeout", s.cfg.Timing.StartTimeout.String())
	}
	if !s.waitFor(false, 0) || !s.pause(s.cfg.Timing.Settle) {
		return
	}
	s.finish(checkBindings(s.cfg.Paths, backup, s.cfg.Now().UTC()))
}

// waitFor polls until the game's running state is want; timeout 0 waits for ever. False when
// the timeout passed or the service is stopping.
func (s *Service) waitFor(want bool, timeout time.Duration) bool {
	deadline := s.cfg.Now().Add(timeout)
	for {
		running, err := s.cfg.Processes.Running(GameImage)
		if err != nil {
			s.cfg.Logger.Warn("process list not read", "error", err)
		} else if running == want {
			return true
		}
		if timeout > 0 && !s.cfg.Now().Before(deadline) {
			return false
		}
		if !s.pause(s.cfg.Timing.Poll) {
			return false
		}
	}
}

// pause sleeps, and is false when the service is stopping.
func (s *Service) pause(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// checkBindings compares controls.ini with its backup and restores the backup on a change.
func checkBindings(paths Paths, backup ControlsBackup, now time.Time) contracts.ControlsCheck {
	check := contracts.ControlsCheck{At: now, BackupSha256: backup.SHA256}
	before, err := os.ReadFile(backup.Path)
	if err != nil {
		return failed(check, fmt.Errorf("read the backup: %w", err))
	}
	current, err := os.ReadFile(paths.Controls())
	if err != nil {
		return failed(check, fmt.Errorf("read controls.ini: %w", err))
	}
	changes := BindingChanges(before, current)
	if len(changes) == 0 {
		check.State = contracts.ControlsCheckStateUnchanged
		return check
	}
	listed := describeChanges(changes)
	if err := RestoreControls(paths, backup, current, now); err != nil {
		return failed(check, fmt.Errorf("%s changed, restore failed: %w", listed, err))
	}
	check.State = contracts.ControlsCheckStateRestored
	check.Detail = &listed
	return check
}

func failed(check contracts.ControlsCheck, err error) contracts.ControlsCheck {
	detail := err.Error()
	check.State = contracts.ControlsCheckStateRestoreFailed
	check.Detail = &detail
	return check
}

func describeChanges(changes []string) string {
	if len(changes) <= maxListedChanges {
		return strings.Join(changes, ", ")
	}
	shown := strings.Join(changes[:maxListedChanges], ", ")
	return fmt.Sprintf("%s and %d more", shown, len(changes)-maxListedChanges)
}

// finish records the check and logs it.
func (s *Service) finish(check contracts.ControlsCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Controls = &check
	if err := s.save(); err != nil {
		s.cfg.Logger.Error("launcher state not saved", "error", err)
	}
	detail := ""
	if check.Detail != nil {
		detail = *check.Detail
	}
	level := s.cfg.Logger.Info
	if check.State == contracts.ControlsCheckStateRestoreFailed {
		level = s.cfg.Logger.Error
	}
	level("wheel bindings checked after the game", "state", string(check.State),
		"detail", detail, "backup_sha256", check.BackupSha256)
}
