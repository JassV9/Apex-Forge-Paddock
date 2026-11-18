package ingest

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// withExtraColumns is the sample's first pass with every channel of docs/adr/0028 filled
// with the sample index.
func withExtraColumns(t *testing.T) *contracts.SpoolPass {
	t.Helper()
	decoder, err := NewDecoder()
	if err != nil {
		t.Fatal(err)
	}
	line, err := decoder.Decode([]byte(samplePass(t)))
	if err != nil {
		t.Fatal(err)
	}
	pass := line.Pass
	c := &pass.Samples.Columns
	for _, column := range []*[]float64{&c.NdFl, &c.NdFr, &c.NdRl, &c.NdRr, &c.WFl, &c.WFr, &c.WRl,
		&c.WRr, &c.StFl, &c.StFr, &c.StRl, &c.StRr, &c.Vx, &c.Vz, &c.X, &c.Z, &c.Gear, &c.Rpm} {
		*column = make([]float64, pass.Samples.N)
		for i := range *column {
			(*column)[i] = float64(i)
		}
	}
	return pass
}

func TestTheNewChannelsAreOptionalColumnsOfTheParquetPart(t *testing.T) {
	newer := withExtraColumns(t)
	older := withExtraColumns(t)
	older.Seq, older.Samples.Columns = 2, sampleColumnsOnly(older.Samples.Columns)
	path := filepath.Join(t.TempDir(), "part.parquet")

	if _, err := writeParquet(path, []*contracts.SpoolPass{newer, older}); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[sampleRow](path)
	if err != nil {
		t.Fatal(err)
	}
	last := newer.Samples.N - 1
	cases := []struct {
		name string
		row  sampleRow
		want *float64
	}{
		{"a pass with the channels", rows[last], &[]float64{float64(last)}[0]},
		{"a pass recorded before them", rows[len(rows)-1], nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, field := range c.row.extraFields() {
				got := *field
				if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
					t.Errorf("%s = %v, want %v", extraColumnNames[k], got, c.want)
				}
			}
		})
	}
}

// oldRow is a reader that knows only the first 24 channels, as core/internal/passes does.
type oldRow struct {
	Seq int32   `parquet:"seq"`
	T   float64 `parquet:"t"`
	RhR float64 `parquet:"rh_r"`
}

func TestAReaderOfTheOlderColumnsStillReadsAPartWithTheNewOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "part.parquet")
	pass := withExtraColumns(t)
	if _, err := writeParquet(path, []*contracts.SpoolPass{pass}); err != nil {
		t.Fatal(err)
	}

	rows, err := parquet.ReadFile[oldRow](path)

	if err != nil || len(rows) != pass.Samples.N || rows[1].T != pass.Samples.Columns.T[1] {
		t.Errorf("old reader: %d rows, err=%v", len(rows), err)
	}
}

func sampleColumnsOnly(c contracts.SampleColumns) contracts.SampleColumns {
	return contracts.SampleColumns{
		T: c.T, D: c.D, V: c.V, Gas: c.Gas, Brake: c.Brake, Steer: c.Steer, LatG: c.LatG,
		LonG: c.LonG, Yaw: c.Yaw, SaFl: c.SaFl, SaFr: c.SaFr, SaRl: c.SaRl, SaRr: c.SaRr,
		SrFl: c.SrFl, SrFr: c.SrFr, SrRl: c.SrRl, SrRr: c.SrRr, LoadFl: c.LoadFl, LoadFr: c.LoadFr,
		LoadRl: c.LoadRl, LoadRr: c.LoadRr, Out: c.Out, RhF: c.RhF, RhR: c.RhR,
	}
}

func TestAnOptionalColumnIsAbsentOrHasEveryValue(t *testing.T) {
	cases := []struct {
		name   string
		length int
		reject bool
	}{
		{"absent", 0, false},
		{"every value", -1, false},
		{"short", 3, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pass := withExtraColumns(t)
			switch {
			case c.length == 0:
				pass.Samples.Columns.Rpm = nil
			case c.length > 0:
				pass.Samples.Columns.Rpm = pass.Samples.Columns.Rpm[:c.length]
			}
			err := checkColumnLengths(pass)
			var rejected *RejectError
			if got := errors.As(err, &rejected) && rejected.Reason == ReasonColumnLength; got != c.reject {
				t.Errorf("rejected = %v (err %v), want %v", got, err, c.reject)
			}
		})
	}
}
