# Case study: nine setup versions that made the car slower

How a setup-advice app I wrote told me every change was an improvement while the car got
worse, what the data showed when I audited it, and what each finding turned into in the
rebuild. Every number here comes from the v0 source, from one real session log (VRC Formula
Alpha 2026 at Circuit of the Americas, 136 corner passes over 17 corners), or from a test in
this repository; the place is named each time. The full audit is
[docs/audit-v0.md](audit-v0.md).

## What happened

Apex Forge v0.3 was a 4,400-line Lua app running inside Assetto Corsa. It recorded how I
drove each corner, scored the car's balance, picked a setup change from a rule table or a
language model, and judged the next version against the one before.

At COTA it took the car from the base setup to version 9: three front anti-roll bar
softenings, five rear bar stiffenings and 5 % of brake bias. The app called the steps
improvements. The car was slower, and I went back to the base setup.

The app's tests passed the whole time. They checked that each change was *legal* (a real
item, on its step grid, inside its range, applied and read back). Nothing checked that a
change was *effective*.

## The three findings

### 1. The verdict fired on noise

My corner times on that session scatter by 28 ms (median per corner; 76 ms in the worst
one). The app compared two versions corner by corner, on as few as three passes each, and
called the newer one a regression when two or more corners were slower and more were slower
than faster.

Run those exact rules on two **identical** setups with that scatter:

```console
$ python tools/verdict_noise_sim.py
n=3: identical setups flagged slowerWins 39%
n=5: identical setups flagged slowerWins 33%
n=8: identical setups flagged slowerWins 26%
```

Seventeen corners tested separately, with no correction for testing seventeen times, will
always produce some "slower" corners. More passes helped a little. No threshold would have
fixed it.

### 2. The diagnosis measured the driver

"Understeer" was front slip angle minus rear slip angle. Split the same session by apex
speed and the reading is +4.2° below 100 km/h (front 7.7°, rear 3.1°), +1.6° between 100
and 150, and +0.7° above 150. Rear slip is flat. Only the front grows, and only in slow
corners.

A balance problem a bar can cure moves both axles. This pattern fits a driver steering past
the front tyre's peak in hairpins, which is how I drive: late on the brakes and trailing
them in. Eight versions of bar and brake changes barely moved the number, because the number
was mostly me. The app never asked whether more steering was still producing more grip.

### 3. Nothing was comparable

Versions were driven one after another, each judged only against its parent, never against
a baseline driven again the same day. A pass carried no time, fuel load, tyre temperature
or track grip, so a track rubbering in, tyres coming up to temperature and me learning the
circuit were all credited to the setup. Version 2 changed many items at once, so nothing
about it could be attributed; versions 3 to 9 were then built on it.

The other five causes in the audit are variations on these: the app optimised its own
balance score instead of lap time, its "push lap" filter selected on the outcome, and its
book of per-click effects never received one usable sample.

## What each finding became

| Finding | In v0 | In Paddock | Where it is tested |
| --- | --- | --- | --- |
| Verdicts fire on noise | Per-corner medians, 3 passes, 17 separate comparisons | One effect over all corners from a mixed model, with an interval; a decision needs the whole interval outside a 5 ms band | `engine/tests/test_stats*.py`: identical setups lead to a decision in 0.0 % of 200 simulated experiments ([phase 2 demo](demos/phase-2.md)) |
| Nobody knew what could be detected | Not asked | The plan states the minimum detectable effect before driving: 14 ms per corner at 6 passes in 10 corners, against 26 ms for v0's 3 passes in 6 | `engine/paddock_engine/stats/mde.py` |
| Versions judged against their parent | Sequential, never re-driven | A-B-A-B blocks against a re-driven baseline, setups under blind code names, the success test fixed before driving | `agent/tests/e2e`: a planted regression is rejected and the baseline put back ([phase 3 demo](demos/phase-3.md)) |
| No conditions recorded | No time, fuel, tyres or grip on a pass | Every pass carries them; the model adjusts for the ones every pass has | `contracts/spool_pass.schema.json` |
| Diagnosis measured the driver | One balance number, one knob | A hypothesis with its evidence and, always, a driver-input and a conditions alternative; an experiment to run, never a fix | `engine/tests/test_diagnosis.py`: on this same session the top reading is slow-corner understeer **with driver input marked as supported by the data** ([phase 4 demo](demos/phase-4.md)) |
| Changes not attributable | Many items per version | One item per experiment; the language model may only put computed evidence into words, and a reply with a number that was not measured is thrown away | `engine/tests/test_explain.py` |
| The effect book never filled | Needed lucky one-change versions | Every finished experiment is a row; the profile is re-derived from them, with intervals; a number from another car is always shown as a prior | `engine/tests/test_sensitivity.py` ([phase 5 demo](demos/phase-5.md)) |
| A tie was a win | "No effect" kept the change | Inconclusive means the baseline stays. Failed experiments stay in the ledger | `core/internal/experiments` |

## What the rebuild caught in itself

The end-to-end test runs the real Lua agent on a mocked game against the real Go service and
Python engine. Twice it found a design error that every unit test had passed:

- After a rejected experiment, the next one would have started from the rejected setup: the
  same mistake as building versions 3 to 9 on version 2.
- A session could be diagnosed while its features were still being computed, and the empty
  answer was stored as if it were the whole one.

Both are fixed, and both are why that test exists.

## What this does not show

- **Nothing here has been run inside Assetto Corsa.** The capture code is ported from v0,
  which did run there; the rebuilt agent is tested against a mocked game only.
- **No real experiment has been driven.** The verdict statistics are validated on
  simulation and on the scatter of one real session. Whether 28 ms holds on other cars,
  tracks and days is not known.
- **The driver-input reading is an alternative, not a proof.** The data speaks for it; one
  session on one setup cannot rule out the car.
- The session log has no conditions, because v0 never recorded them. That is finding 3, and
  it is also why this dataset cannot be re-analysed with the new model's adjustments.

## What I took from it

A test suite proves what it asks. Mine asked whether a change was legal. The question that
mattered was whether I could tell a real effect from two identical setups, and the first
time anything asked it, the answer was no 39 % of the time. The rebuild starts from that
question, and every later piece is allowed to say "I do not know".
