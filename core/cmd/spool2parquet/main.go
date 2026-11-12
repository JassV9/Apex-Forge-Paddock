// Command spool2parquet converts one spool session directory to Parquet parts, exactly as
// core's ingest writes them, without a database.
//
//	go run ./cmd/spool2parquet <spool session dir> <output dir>
//
// It exists to build the engine's test fixture, so the engine is tested on files that the Go
// writer really produced from what the Lua agent really wrote.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: spool2parquet <spool session dir> <output dir>")
		os.Exit(2)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	parts, err := ingest.ConvertSession(os.Args[1], os.Args[2], logger)
	if err != nil {
		logger.Error("conversion failed", "error", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d parquet parts to %s\n", parts, os.Args[2])
}
