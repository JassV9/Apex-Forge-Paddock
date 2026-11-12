# Demo script: phase 4 (diagnosis and explanation)

What phase 4 proves: the session that fooled v0 is now read as "understeer in slow corners,
and the driver's steering explains it as well as the car does"; a language model that makes
something up is not shown; and all of it runs with no model at all. About five minutes.
Needs Docker for part 1, uv for parts 2 and 3; no game, no API key.

## 1. The real session, in the dashboard

```bash
make demo
```

Open <http://localhost:5173>, click **Diagnosis** on the COTA session, then **Diagnose this
session**. Point at:

- **"Understeer mid-corner in slow corners: the front axle slips 7.7 degrees against 3.3 at
  the rear."** This is the reading v0 acted on for eight setup versions.
- Under it, **Driver input: the data speaks for this.** Front slip is 7.7 degrees in slow
  corners and 3.7 in fast ones while rear slip stays near 3. A balance problem would move
  the rear too. Each number is listed with its evidence id and pass count.
- **Conditions** is listed for every hypothesis and never marked as shown: one session on
  one setup cannot separate them.
- "Written from templates, without a language model."
- "None on this car's setup": the recorded session has no setup data, so no item can be
  named. Part 3 shows candidates.

Stop it with `make demo-down`.

## 2. The acceptance tests

```bash
cd engine
uv run pytest tests/test_diagnosis.py tests/test_explain.py -v -k "acceptance or diagnose_endpoint"
```

| Test | What it shows |
| --- | --- |
| `test_acceptance_slow_corner_understeer_at_cota_lists_front_saturation_with_its_evidence` | On the COTA passes the top slow-corner understeer hypothesis has a supported driver-input alternative that cites front and rear slip in slow and fast corners |
| `test_acceptance_a_reply_that_invents_a_metric_or_a_number_is_rejected` (6 cases) | A reply citing `brake_temp_front.slow`, naming `tyre_wear_front` in prose, promising "0.35 seconds", or writing "134 percent" is thrown away with the reason, and the template text is shown |
| `test_diagnose_endpoint_runs_with_no_model_and_matches_the_contract` | `POST /diagnose` with `NullProvider` returns a full diagnosis |

## 3. From a hypothesis to an experiment

```bash
cd engine
uv run pytest tests/test_diagnosis.py -v -k "wings or names_what or inverted or brake_bias"
```

- Fast-corner oversteer is tested with the wings first; slow-corner understeer with the bars.
- A camber item per wheel is listed as not testable ("a left/right pair cannot be changed in
  one experiment yet"), and so is an item at the end of its range.
- A car that stores a value inverted gets the click the other way.
- Brake bias rearward is not proposed while the rear tyres lock.

In the dashboard a candidate has a **Test this** link. It opens the propose form with the
item, the click and the hypothesis filled in; the stored experiment records that its
hypothesis came from the rules, with the evidence and alternatives as they were shown
(`core/internal/experiments/origin_test.go`). `make test-e2e` runs a diagnosis through the
real core and engine before its three experiments.

## 4. With a language model (not verified)

```bash
export GEMINI_API_KEY=...        # or PADDOCK_LLM_PROVIDER=openrouter and OPENROUTER_API_KEY
make dev
```

The explanation then says who wrote it. If the reply fails the gate, the page shows the
template text and the reason. No real call to Gemini or OpenRouter has been made in this
project: the providers are tested against a recorded transport only.

## What is not shown

- Nothing here was run in Assetto Corsa.
- The thresholds of the driver-input alternative are judgement, not measurements.
- Candidates carry a direction, not a size. Sizes come with the sensitivity profile (phase 5).
