package passes

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"
)

// sampleRow is one raw sample as ingest writes it (core/internal/ingest/parquet.go): the
// column names are the spool column names, seq says which pass the sample belongs to.
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

// readPass returns the samples of pass seq from the Parquet parts in dir, in recorded order.
// A pass is written whole into one part, so the search stops at the first part that has it.
// A part whose statistics rule the pass out is not decoded.
func readPass(dir string, seq int) ([]sampleRow, error) {
	parts, err := filepath.Glob(filepath.Join(dir, "*.parquet"))
	if err != nil {
		return nil, fmt.Errorf("list parquet parts: %w", err)
	}
	sort.Strings(parts)
	for _, part := range parts {
		rows, err := readPart(part, int32(seq)) //nolint:gosec // seq is bounded by the schema
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			return rows, nil
		}
	}
	return nil, nil
}

func readPart(path string, seq int32) (rows []sampleRow, err error) {
	file, err := os.Open(path) //nolint:gosec // a part under core's own data directory
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	opened, err := parquet.OpenFile(file, info.Size(), parquet.SkipPageIndex(true), parquet.SkipBloomFilters(true))
	if err != nil {
		return nil, fmt.Errorf("open parquet %s: %w", path, err)
	}
	if !mayHold(opened, seq) {
		return nil, nil
	}
	all, err := parquet.Read[sampleRow](file, info.Size())
	if err != nil {
		return nil, fmt.Errorf("read parquet %s: %w", path, err)
	}
	for _, row := range all {
		if row.Seq == seq {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// mayHold is false only when every row group's statistics of the seq column exclude seq.
// Without statistics the part has to be read.
func mayHold(file *parquet.File, seq int32) bool {
	for i := range file.Metadata().RowGroups {
		low, high, ok := seqBounds(&file.Metadata().RowGroups[i])
		if !ok || (seq >= low && seq <= high) {
			return true
		}
	}
	return false
}

// seqBounds reads the plain-encoded INT32 minimum and maximum of the seq column chunk.
func seqBounds(group *format.RowGroup) (low, high int32, ok bool) {
	for _, column := range group.Columns {
		path := column.MetaData.PathInSchema
		if len(path) != 1 || path[0] != "seq" {
			continue
		}
		stats := column.MetaData.Statistics
		if len(stats.MinValue) != 4 || len(stats.MaxValue) != 4 {
			return 0, 0, false
		}
		low = int32(binary.LittleEndian.Uint32(stats.MinValue))  //nolint:gosec // INT32 bits
		high = int32(binary.LittleEndian.Uint32(stats.MaxValue)) //nolint:gosec // INT32 bits
		return low, high, true
	}
	return 0, 0, false
}

// channels are the row's 24 channel values in the order of contracts.SampleColumns.
func (r *sampleRow) channels() [24]float64 {
	return [24]float64{
		r.T, r.D, r.V, r.Gas, r.Brake, r.Steer, r.LatG, r.LonG, r.Yaw,
		r.SaFl, r.SaFr, r.SaRl, r.SaRr, r.SrFl, r.SrFr, r.SrRl, r.SrRr,
		r.LoadFl, r.LoadFr, r.LoadRl, r.LoadRr, r.Out, r.RhF, r.RhR,
	}
}
