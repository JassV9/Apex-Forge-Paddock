package launcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// GameImage is the Assetto Corsa process.
const GameImage = "acs.exe"

// Processes answers whether a program is running (the Windows process list in production).
type Processes interface {
	Running(image string) (bool, error)
}

// URLOpener hands a URL to the program registered for its scheme (Content Manager here).
type URLOpener interface {
	Open(target string) error
}

// Timing paces the watcher.
type Timing struct {
	// Poll is how often the process list is read.
	Poll time.Duration
	// StartTimeout is how long the game may take to appear after Content Manager is asked.
	StartTimeout time.Duration
	// Settle is how long after the game exits the bindings are compared, so that a save
	// Content Manager makes as the game closes is caught too.
	Settle time.Duration
}

// DefaultTiming polls every three seconds and gives Content Manager three minutes.
var DefaultTiming = Timing{Poll: 3 * time.Second, StartTimeout: 3 * time.Minute, Settle: 5 * time.Second}

// Config is everything a Service needs; Now and Logger default to the clock and a discard log.
type Config struct {
	Paths     Paths
	Version   string
	Processes Processes
	Opener    URLOpener
	Timing    Timing
	// Weather is the Content Manager weather id every launch uses; empty keeps the template's.
	Weather string
	Now     func() time.Time
	Logger  *slog.Logger
}

// Service launches the game and watches each launch until the bindings are checked.
type Service struct {
	cfg     Config
	ctx     context.Context
	mu      sync.Mutex
	state   state
	watches sync.WaitGroup
}

// NewService fills in the defaults of cfg.
func NewService(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	return &Service{cfg: cfg, ctx: context.Background()}
}

// Start loads the remembered state and resumes watching a launch that was not checked yet.
// Watchers stop when ctx is cancelled; the state keeps them for the next start.
func (s *Service) Start(ctx context.Context) error {
	loaded, err := loadState(s.cfg.Paths.StateFile())
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx, s.state = ctx, loaded
	if loaded.watching() {
		s.cfg.Logger.Info("resuming the bindings check of the last launch",
			"backup", loaded.Backup.Path)
		s.watches.Add(1)
		go s.watch(*loaded.Backup, false)
	}
	return nil
}

// Wait returns when every watcher has finished (after the check, or after ctx ended).
func (s *Service) Wait() { s.watches.Wait() }

// Status reports the game, the last launch and the bindings check.
func (s *Service) Status() contracts.LauncherStatus {
	running, err := s.cfg.Processes.Running(GameImage)
	if err != nil {
		s.cfg.Logger.Warn("process list not read", "error", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return contracts.LauncherStatus{
		Service:     contracts.LauncherStatusServiceLauncher,
		Version:     s.cfg.Version,
		GameRunning: running,
		LastLaunch:  s.state.LastLaunch,
		Controls:    s.state.Controls,
	}
}

// Launch backs up the bindings, writes the preset and asks Content Manager to start Practice.
func (s *Service) Launch(request contracts.LaunchRequest) (contracts.LauncherStatus, error) {
	if err := checkInstalled(s.cfg.Paths, request); err != nil {
		return contracts.LauncherStatus{}, err
	}
	if err := s.launchLocked(request); err != nil {
		return contracts.LauncherStatus{}, err
	}
	return s.Status(), nil
}

func (s *Service) launchLocked(request contracts.LaunchRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refuseWhileBusy(); err != nil {
		return err
	}
	preset, err := s.preset(request)
	if err != nil {
		return err
	}
	now := s.cfg.Now().UTC()
	backup, err := BackupControls(s.cfg.Paths, now)
	if err != nil {
		return err
	}
	if err := writeFile(s.cfg.Paths.Preset(), preset); err != nil {
		return err
	}
	previous := s.state
	s.state = launchedState(request, backup, now)
	if err := saveState(s.cfg.Paths.StateFile(), s.state); err != nil {
		s.state = previous
		return err
	}
	if err := s.cfg.Opener.Open(QuickDriveURL(s.cfg.Paths.Preset())); err != nil {
		s.state = previous
		return fmt.Errorf("ask Content Manager to start: %w", joinSaveError(err, s.save()))
	}
	s.cfg.Logger.Info("asked Content Manager to start practice", "car_id", request.CarId,
		"track_id", TrackID(request.TrackId, request.Layout), "backup", backup.Path)
	s.watches.Add(1)
	go s.watch(backup, true)
	return nil
}

func (s *Service) refuseWhileBusy() error {
	if s.state.watching() {
		return fmt.Errorf("%w: the last launch is still being watched until the game exits", ErrBusy)
	}
	running, err := s.cfg.Processes.Running(GameImage)
	if err != nil {
		return fmt.Errorf("read the process list: %w", err)
	}
	if running {
		return fmt.Errorf("%w: Assetto Corsa is already running; leave it first", ErrBusy)
	}
	return nil
}

func (s *Service) preset(request contracts.LaunchRequest) ([]byte, error) {
	template, err := os.ReadFile(s.cfg.Paths.Template)
	if err != nil {
		return nil, fmt.Errorf("read the Quick Drive template %s: %w", s.cfg.Paths.Template, err)
	}
	return BuildPreset(template, request, s.cfg.Weather)
}

func launchedState(request contracts.LaunchRequest, backup ControlsBackup, now time.Time) state {
	return state{
		LastLaunch: &contracts.LaunchRecord{
			CarId: request.CarId, TrackId: request.TrackId, Layout: request.Layout, At: now,
		},
		Controls: &contracts.ControlsCheck{
			State: contracts.ControlsCheckStateWatching, At: now, BackupSha256: backup.SHA256,
		},
		Backup: &backup,
	}
}

func (s *Service) save() error { return saveState(s.cfg.Paths.StateFile(), s.state) }

func joinSaveError(err, saveErr error) error {
	if saveErr == nil {
		return err
	}
	return fmt.Errorf("%w (and the launcher state was not saved: %w)", err, saveErr)
}

// checkInstalled refuses a car or layout that is not there: Content Manager would silently
// start with whatever it had selected before.
func checkInstalled(paths Paths, request contracts.LaunchRequest) error {
	if err := checkFolderName("car", request.CarId); err != nil {
		return err
	}
	if err := checkFolderName("track", request.TrackId); err != nil {
		return err
	}
	if request.Layout != "" {
		if err := checkFolderName("layout", request.Layout); err != nil {
			return err
		}
	}
	if !isCar(paths.carDir(request.CarId)) {
		return fmt.Errorf("%w: car %q is not installed", ErrInvalid, request.CarId)
	}
	if !layoutInstalled(paths.trackDir(request.TrackId), request.Layout) {
		return fmt.Errorf("%w: layout %q is not installed", ErrInvalid,
			TrackID(request.TrackId, request.Layout))
	}
	return nil
}
