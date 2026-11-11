package ingest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// sampleRow is one raw sample in a session's Parquet files. Column names are the spool column
// names; seq and corner say which pass the sample belongs to.
type sampleRow struct {
	Seq    int32   `parquet:"seq"`
	Corner int32   `parquet:"corner"`
	T      float64 `parquet:"t"`
	D      float64 `parquet:"d"`
	V      float64 `parquet:"v"`
	Gas    float64 `parquet:"gas"`
	Brake  float64 `parquet:"brake"`
	Steer  float64 `parquet:"steer"`
	LatG   float64 `parquet:"lat_g"`
	LonG   float64 `parquet:"lon_g"`
	Yaw    float64 `parquet:"yaw"`
	SaFl   float64 `parquet:"sa_fl"`
	SaFr   float64 `parquet:"sa_fr"`
	SaRl   float64 `parquet:"sa_rl"`
	SaRr   float64 `parquet:"sa_rr"`
	SrFl   float64 `parquet:"sr_fl"`
	SrFr   float64 `parquet:"sr_fr"`
	SrRl   float64 `parquet:"sr_rl"`
	SrRr   float64 `parquet:"sr_rr"`
	LoadFl float64 `parquet:"load_fl"`
	LoadFr float64 `parquet:"load_fr"`
	LoadRl float64 `parquet:"load_rl"`
	LoadRr float64 `parquet:"load_rr"`
	Out    float64 `parquet:"out"`
	RhF    float64 `parquet:"rh_f"`
	RhR    float64 `parquet:"rh_r"`
}

var sampleColumnNames = []string{
	"t", "d", "v", "gas", "brake", "steer", "lat_g", "lon_g", "yaw",
	"sa_fl", "sa_fr", "sa_rl", "sa_rr", "sr_fl", "sr_fr", "sr_rl", "sr_rr",
	"load_fl", "load_fr", "load_rl", "load_rr", "out", "rh_f", "rh_r",
}

// sampleColumns lists a pass's columns in the order of sampleColumnNames.
func sampleColumns(c *contracts.SampleColumns) [][]float64 {
	return [][]float64{
		c.T, c.D, c.V, c.Gas, c.Brake, c.Steer, c.LatG, c.LonG, c.Yaw,
		c.SaFl, c.SaFr, c.SaRl, c.SaRr, c.SrFl, c.SrFr, c.SrRl, c.SrRr,
		c.LoadFl, c.LoadFr, c.LoadRl, c.LoadRr, c.Out, c.RhF, c.RhR,
	}
}

func rowAt(pass *contracts.SpoolPass, i int) sampleRow {
	c := &pass.Samples.Columns
	return sampleRow{
		Seq: int32(pass.Seq), Corner: int32(pass.Corner), //nolint:gosec // bounded by the schema
		T: c.T[i], D: c.D[i], V: c.V[i], Gas: c.Gas[i], Brake: c.Brake[i], Steer: c.Steer[i],
		LatG: c.LatG[i], LonG: c.LonG[i], Yaw: c.Yaw[i],
		SaFl: c.SaFl[i], SaFr: c.SaFr[i], SaRl: c.SaRl[i], SaRr: c.SaRr[i],
		SrFl: c.SrFl[i], SrFr: c.SrFr[i], SrRl: c.SrRl[i], SrRr: c.SrRr[i],
		LoadFl: c.LoadFl[i], LoadFr: c.LoadFr[i], LoadRl: c.LoadRl[i], LoadRr: c.LoadRr[i],
		Out: c.Out[i], RhF: c.RhF[i], RhR: c.RhR[i],
	}
}

func sampleRows(passes []*contracts.SpoolPass) []sampleRow {
	var rows []sampleRow
	for _, pass := range passes {
		if pass.Samples == nil {
			continue
		}
		for i := range pass.Samples.N {
			rows = append(rows, rowAt(pass, i))
		}
	}
	return rows
}

// writeParquet writes the raw samples of the given passes as one Parquet part and returns the
// number of rows. Nothing is written when no pass has samples. The file appears atomically:
// a reader never sees a half-written part, and re-ingesting a chunk rewrites the same part.
func writeParquet(path string, passes []*contracts.SpoolPass) (int, error) {
	rows := sampleRows(passes)
	if len(rows) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return 0, fmt.Errorf("create parquet directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := parquet.WriteFile(temporary, rows, parquet.Compression(&zstd.Codec{})); err != nil {
		return 0, fmt.Errorf("write parquet %s: %w", temporary, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return 0, fmt.Errorf("publish parquet %s: %w", path, err)
	}
	return len(rows), nil
}
