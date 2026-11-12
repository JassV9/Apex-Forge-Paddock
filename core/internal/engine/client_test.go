package engine

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func planRequest() contracts.PlanRequest {
	return contracts.PlanRequest{
		SuccessTest: contracts.SuccessTest{
			Channel: "corner_time_s", Rope: 0.005, Level: 0.9, Better: contracts.SuccessTestBetterLower,
		},
		Corners:        []int{1, 2},
		LapsPerBlock:   3,
		BaselinePasses: []contracts.PlanPass{},
	}
}

func TestPlanPostsTheContractAndDecodesTheReply(t *testing.T) {
	var got contracts.PlanRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/plan" {
			t.Errorf("engine was called with %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"sigma":0.028,"sigma_source":"assumed","corners":[1,2],
			"blocks":[{"order":1,"arm":"baseline","laps_target":3}],
			"passes_per_arm":6,"mde_detect":0.03,"mde_decide":0.05}`))
	}))
	defer server.Close()

	reply, err := New(server.URL+"/").Plan(t.Context(), planRequest())

	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got.LapsPerBlock != 3 || len(got.Corners) != 2 {
		t.Errorf("engine received %+v", got)
	}
	if reply.PassesPerArm != 6 || reply.Blocks[0].Arm != contracts.PlanResponseBlocksElemArmBaseline {
		t.Errorf("reply = %+v", reply)
	}
}

func TestFailuresSayWhetherAskingAgainCanHelp(t *testing.T) {
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"bad passes"}`, http.StatusUnprocessableEntity)
	}))
	defer refusing.Close()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer failing.Close()
	garbled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sigma":"not a number"}`))
	}))
	defer garbled.Close()
	stopped := httptest.NewServer(http.NotFoundHandler())
	stopped.Close()

	cases := map[string]struct {
		baseURL string
		want    error
	}{
		"not configured":   {"", ErrUnavailable},
		"connection fails": {stopped.URL, ErrUnavailable},
		"engine error":     {failing.URL, ErrUnavailable},
		"garbled reply":    {garbled.URL, ErrUnavailable},
		"request refused":  {refusing.URL, ErrRejected},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := New(tc.baseURL).Plan(t.Context(), planRequest())
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
