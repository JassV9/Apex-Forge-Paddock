// Package engine is core's HTTP client for the Python analysis engine. Core is the engine's
// only caller; every body is a generated contract type.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// ErrUnavailable means the engine gave no usable answer and asking again later may work: it
// is not configured, unreachable, failed (5xx) or sent a reply that could not be read.
var ErrUnavailable = errors.New("engine unavailable")

// ErrRejected means the engine understood the request and refused it (4xx). Asking again
// with the same request gives the same answer.
var ErrRejected = errors.New("engine rejected the request")

const (
	requestTimeout = 60 * time.Second
	maxErrorBody   = 2048
)

// Client calls one engine. The zero base URL means "no engine configured".
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client for the engine at baseURL, for example "http://engine:8000".
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: requestTimeout},
	}
}

// Features computes the features of a session's corner passes.
func (c *Client) Features(ctx context.Context, request contracts.FeaturesRequest) (contracts.FeaturesResponse, error) {
	return post[contracts.FeaturesResponse](ctx, c, "/features/session", request)
}

// Plan asks what an A-B-A-B run plan can detect.
func (c *Client) Plan(ctx context.Context, request contracts.PlanRequest) (contracts.PlanResponse, error) {
	return post[contracts.PlanResponse](ctx, c, "/plan", request)
}

// Evaluate judges a driven experiment.
func (c *Client) Evaluate(ctx context.Context, request contracts.EvaluateRequest) (contracts.EvaluateResponse, error) {
	return post[contracts.EvaluateResponse](ctx, c, "/evaluate", request)
}

// Diagnose asks what a session's passes say about the car.
func (c *Client) Diagnose(ctx context.Context, request contracts.DiagnoseRequest) (contracts.Diagnosis, error) {
	return post[contracts.Diagnosis](ctx, c, "/diagnose", request)
}

// Sensitivity pools a car's measured effects and borrowed priors into its profile.
func (c *Client) Sensitivity(ctx context.Context, request contracts.SensitivityRequest) (contracts.SensitivityResponse, error) {
	return post[contracts.SensitivityResponse](ctx, c, "/sensitivity/update", request)
}

// Propose ranks candidate experiments by what the profile expects of them.
func (c *Client) Propose(ctx context.Context, request contracts.ProposeRequest) (contracts.Proposals, error) {
	return post[contracts.Proposals](ctx, c, "/propose", request)
}

func post[T any](ctx context.Context, c *Client, path string, body any) (T, error) {
	var reply T
	if c.baseURL == "" {
		return reply, fmt.Errorf("PADDOCK_ENGINE_URL is not set: %w", ErrUnavailable)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return reply, fmt.Errorf("encode %s request: %w", path, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return reply, fmt.Errorf("build %s request: %w", path, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return reply, fmt.Errorf("call engine %s: %w: %w", path, ErrUnavailable, err)
	}
	defer response.Body.Close() //nolint:errcheck // nothing to do about a failed close of a read body
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
		kind := ErrUnavailable
		if response.StatusCode < http.StatusInternalServerError {
			kind = ErrRejected
		}
		return reply, fmt.Errorf("engine %s answered %d: %s: %w",
			path, response.StatusCode, strings.TrimSpace(string(detail)), kind)
	}
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		return reply, fmt.Errorf("decode engine %s reply: %w: %w", path, ErrUnavailable, err)
	}
	return reply, nil
}
