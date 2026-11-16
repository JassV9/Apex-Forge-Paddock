package passes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func TestMayHoldUsesTheSeqStatistics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "000002.parquet")
	rows := []sampleRow{{Seq: 4, T: 0}, {Seq: 4, T: 0.02}, {Seq: 5, T: 0}}
	if err := parquet.WriteFile(path, rows); err != nil {
		t.Fatalf("write part: %v", err)
	}
	file, err := os.Open(path) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatalf("open part: %v", err)
	}
	defer file.Close() //nolint:errcheck // read-only
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat part: %v", err)
	}
	opened, err := parquet.OpenFile(file, info.Size())
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}

	for seq, want := range map[int32]bool{3: false, 4: true, 5: true, 6: false} {
		if got := mayHold(opened, seq); got != want {
			t.Errorf("mayHold(%d) = %v, want %v", seq, got, want)
		}
	}
	got, err := readPass(filepath.Dir(path), 4)
	if err != nil || len(got) != 2 || got[1].T != 0.02 {
		t.Errorf("readPass(4) = %+v, %v", got, err)
	}
}
