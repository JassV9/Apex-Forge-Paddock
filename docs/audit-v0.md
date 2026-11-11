# Audit of Apex Forge v0.3.0

Why Paddock exists: what the prototype did, what is kept, and why its setup iterations
regressed. This is section A of the design document (`docs/design.md`), kept here as the
standalone record that the code and ADRs refer to. Claims are backed by the v0 source or by
the real FA26 COTA session log (`agent/tests/fixtures/real_fa26_cota_log.json`, 136 passes).

Reproduce the 39% false-verdict figure:

    python tools/verdict_noise_sim.py

    n=3: identical setups flagged slowerWins 39%
    n=5: identical setups flagged slowerWins 33%
    n=8: identical setups flagged slowerWins 26%


Apex Forge v0.3.0 is a 4,400-line CSP Lua app (`src/core` pure Lua, `src/game` CSP shell, `src/ui` windows) with 9 offline test suites run under LuaJIT via lupa against real AI lines and Jason's real setup exports. It has been driven on the FA26 (COTA, Singapore, Monaco, Mexico, Sepang) and FA25 (COTA). Sensitivity learning (v0.3.0) has never produced a sample.

## What exists, and what happens to it

| Component | What it does today | State | Decision |
| --- | --- | --- | --- |
| `corners.lua` | Corner map from the AI line (radius < 280 m, zones for braking and traction) | Working, tested on real lines | **Retain**, port the algorithm to Python as the canonical version |
| `recorder.lua` | Per-corner pass capture; drops passes on restart, teleport, pits, replay | Working; the right idea for a hot-lapper | **Retain** in the in-game agent |
| `carlink.lua` | Reads per-frame car state into a reused table | Working, cheap | **Improve**: add wall clock, fuel, tyre core temps, wear, track grip, air/road temp to every sample |
| `analyze.lua` | Phase cuts, braking/throttle/steering metrics, front-minus-rear slip balance | Metrics fine; balance is the weak point | **Replace** the balance diagnosis (see below); keep the input metrics |
| `learner.lua` | Push filter, per-corner median and MAD per setup, `compare` | Statistically unsound for verdicts | **Replace** with the experiment engine |
| `outcome.lua` | Improvement / Trade-off / No effect / Regression | Counts corners, ignores magnitudes | **Replace** |
| `sensitivity.lua` | Per-click effect book per knob | Never measured anything | **Replace** with the sensitivity model in section E |
| `advisor.lua` | Rule table: one knob per symptom, fixed order | Rules of thumb with no interaction model | **Refactor** into a hypothesis generator that emits ranked candidates, not answers |
| `catalog.lua`, `reply.lua`, `game/setup.lua`, `versions.lua` | Real labels, ranges, steps; LLM reply parsing; apply with read-back and rollback; never overwrite | The strongest part of the app | **Retain**; make the setup schema the contract between agent and backend |
| `prompt.lua`, `gemini.lua`, `openrouter.lua`, `game/llm.lua` | LLM pack and in-game calls | Works; the LLM is allowed to propose multi-item changes | **Move** to the backend behind a provider interface; LLM explains and ranks, never decides |
| `style.lua` | Driver style per corner | Useful, descriptive only | **Improve** into the driver model |
| `game/app.lua` (767 lines), `ui/main.lua` (477 lines) | Orchestration and all UI | God files | **Refactor**: the in-game UI shrinks to status, experiment prompts and apply |
| `tests/` (mock CSP, lupa, real fixtures, review scripts) | Offline verification | Excellent | **Retain**; becomes the agent's contract tests |

## Why setup iterations regressed

The project history (base to v9 at COTA in the FA26, then back to base) shows the failure end to end. Each cause below is backed by the code or Jason's data.

1. **The diagnosis was wrong, not the fix.** Balance = load-weighted front slip angle minus rear slip angle, scaled by `FRICTION_LIMIT_ANGLE`. On the real COTA log (136 passes, one setup), confident mid-corner readings split by apex speed: below 100 km/h **+4.2°** (front 7.7°, rear 3.1°), 100–150 km/h **+1.6°**, 150–220 km/h **+0.7°**. Rear slip is flat; only front slip grows at low speed. That pattern fits a driver steering past the front tyre's peak in hairpins (Jason is entry-driven and trail-brakes), plus Ackermann and inner-wheel effects at large steering angles. It does not fit a balance problem a bar can cure, and three front-bar softenings, five rear-bar stiffenings and 5% brake bias barely moved it.
2. **Nothing checked whether the front was actually short of grip.** The app never asks "did more steering produce more lateral g?" A saturated front (steering up, lateral g flat) and a car that simply runs more front slip look identical in this metric. The calibrate buttons subtract one constant per group, but the offset is speed-dependent, so the zero is wrong in most corners.
3. **The verdict rules fire on noise.** Per-corner push-pass scatter on the real log is 28 ms median, 76 ms worst. `learner.compare` uses median and MAD from as few as 3 passes, so the noise estimate itself is unstable. Simulating the exact rules on that scatter, identical setups trigger "2+ slower and more slower than faster" in **39%** of comparisons at 3 passes, **33%** at 5 and **26%** at 8 (17 corners, Gaussian noise; script in the project folder). Testing 17 corners separately with no correction for multiple comparisons guarantees false "slower" corners.
4. **It optimized a proxy.** `outcome.evaluate` calls a version an Improvement when the targeted symptom score drops by 0.5, unless 2+ corners got slower. A change that makes the balance number look better while costing lap time across many corners by less than the noise floor each passes as a success.
5. **No comparable conditions.** Versions were driven in sequence, never interleaved, and each was judged only against its parent. Pass records carry no timestamp, fuel, tyre temperature or grip, so drift from rubbering-in, tyre heat or Jason learning COTA is credited to the setup. v3–v9 were built on v2, which was itself a regression.
6. **Changes were not attributable.** v2 (OpenRouter "COTA Race Base") changed many items at once. The reply checker proves a value is *legal*, not that it is *effective*, which is exactly the gap Jason described.
7. **The push filter is a selection bias.** A pass counts only within 5% of that setup's own best, at 85% of its best lateral g. On a 10 s corner that window is about 500 ms wide, twenty times the effects being measured, and a single fluke best tightens the filter for one setup but not the other.
8. **The sensitivity book could not fill.** It needs one-change versions with 3+ corners at 3+ push passes on both sides; none of v3, v5, v6 or v9 qualified. Its 0.25° noise constant is a guess, and it compares car-wide group means instead of paired corner deltas.

## What is overcomplicated

- Rich pass analytics running inside the game thread in LuaJIT, with per-track JSON files up to about 1 MB rewritten whole. Analysis belongs out of process.
- Five verdict categories built on hard thresholds. One posterior estimate of lap-time effect with an interval, plus a few guardrail metrics, says more with less code.
- Two in-game LLM clients and a model picker. One backend provider interface is enough.

## What is genuinely good and must survive

Per-corner capture that tolerates restarts; reading roles from in-game labels (the FA25 WING\_1/WING\_2 lesson); step-grid and range validation; apply with read-back and full rollback; never overwriting setups; offline tests against real data; honest "not verified in game" lists. These are the foundation, and they are also good interview material.
