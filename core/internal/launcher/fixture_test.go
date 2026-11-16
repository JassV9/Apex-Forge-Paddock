package launcher

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	// templatePreset is the shape of a real Content Manager Practice preset, shortened.
	templatePreset = "\xef\xbb\xbf" + `{"Mode":"/Pages/Drive/QuickDrive_Practice.xaml",` +
		`"ModeData":"{\"StartType\":\"START\",\"Penalties\":false,\"PlayerBallast\":0}",` +
		`"CarId":"lotus_exos_125_s1","TrackId":"shuto/tatsumi_pa","Temperature":26.0,` +
		`"AssistsData":"{\"Abs\":1,\"TractionControl\":1}","Time":43200}`
	controlsText = "[STEER]\nJOY=0\nAXLE=0\n\n[CONTROLLERS]\nCON0=R3 and Pedals\n" +
		"PGUID0=0005346E\nCON1=Logitech G29\n\n[GEARUP]\nJOY=1\nJOY_MODIFICATOR=-1\n"
)

// writeTestFile creates parents and fails the test on error.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// newFixture lays out an AC install with one car and one track (a layout and its test copy),
// a Documents folder with controls.ini, and a Quick Drive template.
func newFixture(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	paths := Paths{
		ACRoot:    filepath.Join(root, "assettocorsa"),
		Documents: filepath.Join(root, "Documents"),
		Template:  filepath.Join(root, "NJ.cmpreset"),
	}
	writeTestFile(t, filepath.Join(paths.carDir("ks_ferrari_488_gt3"), "ui", "ui_car.json"),
		`{"name": "Ferrari 488 GT3"}`)
	track := paths.trackDir("fn_cota")
	for _, layout := range []string{"gp", "gp_paddock"} {
		writeTestFile(t, filepath.Join(track, "ui", layout, "ui_track.json"),
			`{"name": "Circuit of the Americas - `+layout+`"}`)
		writeTestFile(t, filepath.Join(track, layout, "data", "surfaces.ini"), "[SURFACE_0]\n")
	}
	writeTestFile(t, paths.Controls(), controlsText)
	writeTestFile(t, paths.Template, templatePreset)
	return paths
}

// fakeProcesses is a scripted process list.
type fakeProcesses struct {
	mu      sync.Mutex
	running bool
	err     error
}

func (f *fakeProcesses) Running(string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running, f.err
}

func (f *fakeProcesses) set(running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = running
}

// fakeOpener records URLs and runs onOpen as if Content Manager reacted.
type fakeOpener struct {
	urls   []string
	err    error
	onOpen func()
}

func (f *fakeOpener) Open(target string) error {
	f.urls = append(f.urls, target)
	if f.err == nil && f.onOpen != nil {
		f.onOpen()
	}
	return f.err
}

var fastTiming = Timing{Poll: time.Millisecond, StartTimeout: time.Second, Settle: time.Millisecond}

var fixedNow = time.Date(2026, 11, 16, 18, 30, 0, 0, time.UTC)

// steppingClock starts at fixedNow and moves 100 ms on every reading, so timeouts pass.
func steppingClock() func() time.Time {
	var mu sync.Mutex
	now := fixedNow
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(100 * time.Millisecond)
		return now
	}
}

func newTestService(paths Paths, processes Processes, opener URLOpener) *Service {
	return NewService(Config{
		Paths: paths, Version: "test", Processes: processes, Opener: opener, Timing: fastTiming,
		Now: steppingClock(),
	})
}
