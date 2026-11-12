# Demo script: phase 5 (sensitivity profile and ranking)

What phase 5 proves: the system recovers what each setup item does from planned experiments,
with intervals that contain the truth; a number borrowed from another car is never passed
off as a measurement; and the next experiment it proposes is neither the one already known
to do nothing nor always the one already known to work. About five minutes. Needs uv; part 3
needs Docker's PostgreSQL. No game, no API key.

## 1. A car with known effects is recovered

```bash
cd engine
uv run pytest tests/test_sensitivity.py tests/test_optimize.py -v -k acceptance
```

The simulated car (`tests/sensitivity_data.py`) has a front bar worth 20 ms per click, a
rear bar that does nothing, and a rear wing whose baseline is already the optimum, so a
click either way costs 15 ms. Each change is driven as a planned A-B-A-B experiment and
judged by the same code that judges real ones.

| Test | What it shows |
| --- | --- |
| `test_acceptance_known_effects_are_recovered_within_their_intervals` | All seven true effects lie inside their intervals; the front bar is linear over one and two clicks; the rear bar is classed negligible; the wing is flagged non-linear ("not mirror images") |
| `test_acceptance_a_transferred_effect_is_shown_as_a_prior_never_as_measured` | Another car's 30 ms becomes 15 ms with an interval that still contains both 30 ms and zero, flagged `is_prior`, named after its source, with no experiments and no passes |
| `test_acceptance_a_knob_measured_as_negligible_is_not_proposed` | The rear bar is left out with the reason; the stiffer front bar, measured as slower, too; the softer front bar is ranked first and a camber change nobody measured is ranked because nothing is known about it |
| `test_acceptance_a_negligible_prior_does_not_rule_a_knob_out` | Another car ignoring its rear bar does not stop this car from testing its own |

The driver in this simulation scatters 8 ms per corner pass. A human scatters about 28 ms,
and then "negligible" against a 5 ms band takes about 90 laps per setup: the profile says
"inconclusive" and keeps the change in the ranking. That is the case for the Ghost Driver.

## 2. What core adds

```bash
cd core
go test ./internal/sensitivity/ -v -run "Prior|LeftOut|Lends|Scorecard"
```

- An experiment still being driven, an aborted one, a champion re-check and one judged at
  another interval level are not measurements of one item, and are left out.
- A car whose front bar is called `ARB_F` and whose menu runs the other way borrows the
  Mazda's `ARB_FRONT` effect as a prior, with the click restated; a left and a right damper
  lend nothing.
- The scorecard counts an extended experiment's predictions once, from its newest verdict.

## 3. End to end, and in the dashboard

```bash
make test-e2e
```

The Lua agent on the mocked game, core and the engine run three experiments; afterwards the
profile reads the softer front bar as faster, the stiffer one as slower and the rear bar as
inconclusive, the ranking rules out only the change measured as slower, and the scorecard
has three judged experiments.

With `make dev` and a recorded session:

- **Garage** lists each car's effects with interval bars. A prior is grey, dashed and worded
  "Prior from X, not measured on this car".
- A session's **Diagnosis** page has **What to test next**: the candidates in order, each
  with its expected gain, interval, laps and basis. **Propose this** opens the form with the
  prediction filled in when the car's own measurement expects a gain.
- The **Ledger** starts with the scorecard: how many predictions came true, by where the
  hypothesis came from.

The public demo dataset (`make demo`) has no setup data, so its Garage shows the car with
nothing measured and its diagnosis has no candidates to rank.

## Not shown, because it does not exist

- Anything in Assetto Corsa.
- The surrogate model with Thompson sampling for cars with 20 or more experiments
  ([ADR 0011](../adr/0011-sensitivity-and-ranking.md) says what is ranked instead).
- A profile per track or per conditions.
