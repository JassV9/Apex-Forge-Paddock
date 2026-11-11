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

// Line is one decoded spool line: exactly one of Session and Pass is set.
type Line struct {
	Session *contracts.SpoolSession
	Pass    *contracts.SpoolPass
}

// Decoder validates spool lines against the contracts and decodes them.
type Decoder struct {
	session *jsonschema.Schema
	pass    *jsonschema.Schema
}

// NewDecoder compiles the embedded schemas.
func NewDecoder() (*Decoder, error) {
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"spool_session.schema.json", "spool_pass.schema.json"} {
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
	session, err := compiler.Compile("spool_session.schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile session schema: %w", err)
	}
	pass, err := compiler.Compile("spool_pass.schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile pass schema: %w", err)
	}
	return &Decoder{session: session, pass: pass}, nil
}

// Decode validates one line. Every error it returns is a *RejectError.
func (d *Decoder) Decode(raw []byte) (Line, error) {
	var head struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return Line{}, reject(ReasonInvalidJSON, "%v", err)
	}
	switch head.Kind {
	case "session":
		var session contracts.SpoolSession
		if err := decodeInto(d.session, raw, &session); err != nil {
			return Line{}, err
		}
		return Line{Session: &session}, nil
	case "pass":
		var pass contracts.SpoolPass
		if err := decodeInto(d.pass, raw, &pass); err != nil {
			return Line{}, err
		}
		if err := checkColumnLengths(&pass); err != nil {
			return Line{}, err
		}
		return Line{Pass: &pass}, nil
	default:
		return Line{}, reject(ReasonUnknownKind, "kind %q", head.Kind)
	}
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
