package launcher

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

var cota = contracts.LaunchRequest{CarId: "ks_ferrari_488_gt3", TrackId: "fn_cota", Layout: "gp_paddock"}

func TestLaunchRefuses(t *testing.T) {
	tests := []struct {
		name    string
		request contracts.LaunchRequest
		running bool
		want    error
	}{
		{"car not installed", contracts.LaunchRequest{CarId: "nope", TrackId: "fn_cota", Layout: "gp"}, false, ErrInvalid},
		{"layout not installed", contracts.LaunchRequest{CarId: cota.CarId, TrackId: "fn_cota", Layout: "club"}, false, ErrInvalid},
		{"main layout of a multi-layout track", contracts.LaunchRequest{CarId: cota.CarId, TrackId: "fn_cota"}, false, ErrInvalid},
		{"path in an id", contracts.LaunchRequest{CarId: `..\..\x`, TrackId: "fn_cota", Layout: "gp"}, false, ErrInvalid},
		{"game already running", cota, true, ErrBusy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newFixture(t)
			opener := &fakeOpener{}
			service := newTestService(paths, &fakeProcesses{running: tt.running}, opener)
			if _, err := service.Launch(tt.request); !errors.Is(err, tt.want) {
				t.Fatalf("Launch error = %v, want %v", err, tt.want)
			}
			if len(opener.urls) != 0 || exists(paths.Preset()) || exists(paths.Backups()) {
				t.Error("a refused launch wrote files or asked Content Manager")
			}
		})
	}
}

// launchAndExit launches, lets onGame run while the game "runs", and waits for the check.
func launchAndExit(t *testing.T, paths Paths, onGame func()) *Service {
	t.Helper()
	processes := &fakeProcesses{}
	opener := &fakeOpener{onOpen: func() {
		processes.set(true)
		onGame()
	}}
	service := newTestService(paths, processes, opener)
	status, err := service.Launch(cota)
	if err != nil {
		t.Fatal(err)
	}
	if status.Controls == nil || status.Controls.State != contracts.ControlsCheckStateWatching {
		t.Fatalf("status after launch = %+v, want watching", status.Controls)
	}
	if _, err := service.Launch(cota); !errors.Is(err, ErrBusy) {
		t.Errorf("a second launch while watching: %v, want ErrBusy", err)
	}
	if len(opener.urls) != 1 || opener.urls[0] != QuickDriveURL(paths.Preset()) {
		t.Errorf("opened %v", opener.urls)
	}
	processes.set(false)
	service.Wait()
	return service
}

func TestLaunchWritesThePresetAndFindsBindingsUnchanged(t *testing.T) {
	paths := newFixture(t)
	service := launchAndExit(t, paths, func() {})

	fields, modeData := decodePreset(t, []byte(readTestFile(t, paths.Preset())))
	if fields["CarId"] != cota.CarId || fields["TrackId"] != "fn_cota/gp_paddock" || modeData["StartType"] != "PIT" {
		t.Errorf("preset = %v %v", fields, modeData)
	}
	status := service.Status()
	if status.Controls.State != contracts.ControlsCheckStateUnchanged || status.LastLaunch.Layout != "gp_paddock" {
		t.Errorf("status = %+v / %+v", status.Controls, status.LastLaunch)
	}
	if readTestFile(t, paths.Template) != templatePreset {
		t.Error("the template was changed")
	}
}

func TestLaunchRestoresBindingsTheGameChanged(t *testing.T) {
	paths := newFixture(t)
	changed := strings.Replace(controlsText, "CON1=Logitech G29", "CON1=", 1)
	service := launchAndExit(t, paths, func() { writeTestFile(t, paths.Controls(), changed) })

	if readTestFile(t, paths.Controls()) != controlsText {
		t.Error("controls.ini was not restored")
	}
	check := service.Status().Controls
	if check.State != contracts.ControlsCheckStateRestored || check.Detail == nil || *check.Detail != "CONTROLLERS/CON1" {
		t.Errorf("check = %+v", check)
	}
}

func TestLaunchKeepsOtherChangesWhenBindingsAreTheSame(t *testing.T) {
	paths := newFixture(t)
	tweaked := controlsText + "\n[FF_TWEAKS]\nGAIN=80\n"
	launchAndExit(t, paths, func() { writeTestFile(t, paths.Controls(), tweaked) })

	if readTestFile(t, paths.Controls()) != tweaked {
		t.Error("a change that is not a binding was undone")
	}
}

func TestOpenerFailureLeavesNothingWatched(t *testing.T) {
	paths := newFixture(t)
	service := newTestService(paths, &fakeProcesses{}, &fakeOpener{err: errors.New("no handler")})
	if _, err := service.Launch(cota); err == nil {
		t.Fatal("want an error when Content Manager cannot be asked")
	}
	if status := service.Status(); status.LastLaunch != nil || status.Controls != nil {
		t.Errorf("status = %+v, want no launch", status)
	}
}

func TestGameThatNeverStartsIsCheckedAfterTheTimeout(t *testing.T) {
	paths := newFixture(t)
	service := newTestService(paths, &fakeProcesses{}, &fakeOpener{})
	if _, err := service.Launch(cota); err != nil {
		t.Fatal(err)
	}
	service.Wait()
	if state := service.Status().Controls.State; state != contracts.ControlsCheckStateUnchanged {
		t.Errorf("state = %s, want unchanged", state)
	}
}

func TestStoppedLauncherResumesTheCheck(t *testing.T) {
	paths := newFixture(t)
	processes := &fakeProcesses{}
	ctx, stop := context.WithCancel(context.Background())
	first := newTestService(paths, processes, &fakeOpener{onOpen: func() { processes.set(true) }})
	if err := first.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Launch(cota); err != nil {
		t.Fatal(err)
	}
	stop()
	first.Wait()
	writeTestFile(t, paths.Controls(), strings.Replace(controlsText, "JOY=1", "JOY=4", 1))
	processes.set(false)

	second := newTestService(paths, processes, &fakeOpener{})
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	second.Wait()
	if state := second.Status().Controls.State; state != contracts.ControlsCheckStateRestored {
		t.Errorf("state = %s, want restored", state)
	}
	if readTestFile(t, paths.Controls()) != controlsText {
		t.Error("controls.ini was not restored after the restart")
	}
}
