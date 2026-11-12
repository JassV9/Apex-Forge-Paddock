// Package ingest turns the agent's spool files into corner_pass rows and Parquet files of raw
// samples. It validates and stores; it computes no engineering metrics (that is the engine's
// job) and never writes to the spool directory.
package ingest

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Copies of contracts/*.schema.json, refreshed by `make gen`.
//
//go:embed schemas/*.schema.json
var schemaFiles embed.FS

// Reasons a spool line is rejected. They are the values of the "reason" metric label.
const (
	ReasonInvalidJSON     = "invalid_json"
	ReasonUnknownKind     = "unknown_kind"
	ReasonSchema          = "schema"
	ReasonColumnLength    = "column_length"
	ReasonSessionMismatch = "session_mismatch"
	ReasonUnknownSession  = "unknown_session"
	ReasonUnknownCorner   = "unknown_corner"
)

// RejectError says why one spool line was not ingested.
type RejectError struct {
	Reason string
	Detail string
}

func (e *RejectError) Error() string { return e.Reason + ": " + e.Detail }

func reject(reason, format string, args ...any) *RejectError {
	return &RejectError{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// Line is one decoded spool line: exactly one field is set.
type Line struct {
	Session *contracts.SpoolSession
	Catalog *contracts.SpoolCatalog
	Setup   *contracts.SpoolSetup
	Pass    *contracts.SpoolPass
}

// SessionID is the session the line says it belongs to.
func (l Line) SessionID() string {
	switch {
	case l.Session != nil:
		return l.Session.SessionId
	case l.Catalog != nil:
		return l.Catalog.SessionId
	case l.Setup != nil:
		return l.Setup.SessionId
	default:
		return l.Pass.SessionId
	}
}

// schemaOfKind maps a line's "kind" to the contract it must satisfy.
var schemaOfKind = map[string]string{
	"session": "spool_session.schema.json",
	"catalog": "spool_catalog.schema.json",
	"setup":   "spool_setup.schema.json",
	"pass":    "spool_pass.schema.json",
}

// Decoder validates spool lines against the contracts and decodes them.
type Decoder struct {
	schemas map[string]*jsonschema.Schema // by line kind
}

// NewDecoder compiles the embedded schemas.
func NewDecoder() (*Decoder, error) {
	compiler := jsonschema.NewCompiler()
	for _, name := range schemaOfKind {
		raw, err := schemaFiles.ReadFile("schemas/" + name)
		if err != nil {
			return nil, fmt.Errorf("read embedded schema %s: %w", name, err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("parse schema %s: %w", name, err)
		}
		if err := compiler.AddResource(name, doc); err != nil {
			return nil, fmt.Errorf("add schema %s: %w", name, err)
		}
	}
	schemas := make(map[string]*jsonschema.Schema, len(schemaOfKind))
	for kind, name := range schemaOfKind {
		schema, err := compiler.Compile(name)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}
		schemas[kind] = schema
	}
	return &Decoder{schemas: schemas}, nil
}

// Decode validates one line. Every error it returns is a *RejectError.
func (d *Decoder) Decode(raw []byte) (Line, error) {
	var head struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return Line{}, reject(ReasonInvalidJSON, "%v", err)
	}
	schema, known := d.schemas[head.Kind]
	if !known {
		return Line{}, reject(ReasonUnknownKind, "kind %q", head.Kind)
	}
	line := Line{}
	var target any
	switch head.Kind {
	case "session":
		line.Session = &contracts.SpoolSession{}
		target = line.Session
	case "catalog":
		line.Catalog = &contracts.SpoolCatalog{}
		target = line.Catalog
	case "setup":
		line.Setup = &contracts.SpoolSetup{}
		target = line.Setup
	default:
		line.Pass = &contracts.SpoolPass{}
		target = line.Pass
	}
	if err := decodeInto(schema, raw, target); err != nil {
		return Line{}, err
	}
	if line.Pass != nil {
		if err := checkColumnLengths(line.Pass); err != nil {
			return Line{}, err
		}
	}
	return line, nil
}

func decodeInto(schema *jsonschema.Schema, raw []byte, target any) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return reject(ReasonInvalidJSON, "%v", err)
	}
	if err := schema.Validate(instance); err != nil {
		return reject(ReasonSchema, "%s", firstCause(err))
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return reject(ReasonSchema, "%v", err)
	}
	return nil
}

// firstCause is the deepest first validation message: "at '/seq': minimum: got 0, want 1".
func firstCause(err error) string {
	var validation *jsonschema.ValidationError
	if !errors.As(err, &validation) {
		return err.Error()
	}
	for len(validation.Causes) > 0 {
		validation = validation.Causes[0]
	}
	return validation.Error()
}

// The schema cannot say "every column has n values", so it is checked here.
func checkColumnLengths(pass *contracts.SpoolPass) error {
	if pass.Samples == nil {
		return nil
	}
	for index, column := range sampleColumns(&pass.Samples.Columns) {
		if len(column) != pass.Samples.N {
			return reject(ReasonColumnLength, "column %s has %d values, n is %d",
				sampleColumnNames[index], len(column), pass.Samples.N)
		}
	}
	return nil
}
