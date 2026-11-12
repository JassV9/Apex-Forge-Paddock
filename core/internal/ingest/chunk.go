package ingest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// maxLineBytes bounds one spool line. A 40 s pass at 60 Hz is about 400 kB.
const maxLineBytes = 8 << 20

// Chunk is the decoded content of one spool file, ready to store.
type Chunk struct {
	SpoolID    string // session id: the name of the spool sub-directory
	FileName   string
	Header     *contracts.SpoolSession // set when the file starts a session
	Catalog    *contracts.SpoolCatalog // the car's setup menu, once per session
	Setups     []*contracts.SpoolSetup // setups that went on the car, in order
	Passes     []*contracts.SpoolPass
	ParquetURI string         // directory that holds the session's Parquet parts
	Lines      int            // non-empty lines in the file
	Rejects    map[string]int // lines rejected while decoding, by reason
}

// ChunkResult is what storing a chunk did.
type ChunkResult struct {
	Inserted   int            // new corner_pass rows
	Duplicates int            // passes that were already stored
	Rejects    map[string]int // lines the store refused, by reason
}

// Store persists chunks. PostgresStore is the real one.
type Store interface {
	// DoneChunks returns the file names already ingested for a session.
	DoneChunks(ctx context.Context, spoolID string) (map[string]bool, error)
	// StoreChunk saves a chunk and marks its file as ingested, atomically.
	StoreChunk(ctx context.Context, chunk Chunk) (ChunkResult, error)
}

// readChunk decodes a spool file. Lines that fail validation are counted, not fatal.
func (s *Service) readChunk(spoolID, path string) (Chunk, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path comes from listing the spool directory
	if err != nil {
		return Chunk{}, fmt.Errorf("read spool file: %w", err)
	}
	chunk := Chunk{
		SpoolID:    spoolID,
		FileName:   filepath.Base(path),
		ParquetURI: s.parquetDir(spoolID),
		Rejects:    map[string]int{},
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for scanner.Scan() {
		text := bytes.TrimSpace(scanner.Bytes())
		if len(text) == 0 {
			continue
		}
		chunk.Lines++
		s.addLine(&chunk, text)
	}
	if err := scanner.Err(); err != nil {
		return Chunk{}, fmt.Errorf("scan spool file: %w", err)
	}
	return chunk, nil
}

func (s *Service) addLine(chunk *Chunk, text []byte) {
	line, err := s.decoder.Decode(text)
	if err == nil {
		err = checkSession(chunk.SpoolID, line)
	}
	if err != nil {
		reason := ReasonSchema
		if rejected, ok := err.(*RejectError); ok { //nolint:errorlint // Decode returns it unwrapped
			reason = rejected.Reason
		}
		chunk.Rejects[reason]++
		s.logger.Warn("spool line rejected", "session_id", chunk.SpoolID,
			"file", chunk.FileName, "line", chunk.Lines, "error", err.Error())
		return
	}
	switch {
	case line.Session != nil:
		chunk.Header = line.Session
	case line.Catalog != nil:
		chunk.Catalog = line.Catalog
	case line.Setup != nil:
		chunk.Setups = append(chunk.Setups, line.Setup)
	default:
		chunk.Passes = append(chunk.Passes, line.Pass)
	}
}

// A line must belong to the session whose directory it is in.
func checkSession(spoolID string, line Line) error {
	if lineID := line.SessionID(); lineID != spoolID {
		return reject(ReasonSessionMismatch, "line is for session %q", lineID)
	}
	return nil
}

func (s *Service) parquetDir(spoolID string) string {
	return filepath.Join(s.cfg.DataDir, "sessions", spoolID)
}

func parquetPartName(chunkFile string) string {
	return strings.TrimSuffix(chunkFile, filepath.Ext(chunkFile)) + ".parquet"
}
