# Demo script: phase 6 (AI test driver, setup desk, telemetry workspace)

What phase 6 shows: a machine can drive an experiment's laps in Assetto Corsa on the player's
car, with each block's setup loaded and checked by the agent, and the results land in the same
ledger as a human's. Offline parts take five minutes; the in-game part needs the game, CSP and
Content Manager, and about forty minutes of the AI driving.

## 1. Offline

```bash
cd agent && uv run pytest tests/test_ghost.py tests/test_aitune.py tests/test_desk.py -v
```

| Test | What it shows |
| --- | --- |
| `test_the_agent_drives_a_two_block_plan_with_the_ai_in_its_own_session` | Both blocks loaded, driven and tagged; every block starts with the first block's fuel; the AI is kept from its own pit stops; the car is handed back at the end |
| `test_a_reset_by_the_game_retries_the_block` and `..._no_lap_counted...` | A setup the AI cannot drive ends the run with a reason instead of looping |
| `test_free_laps_learn_a_cap_where_the_ai_went_off_and_apply_it` | A cap where the car went off, applied on track, on the car's own grip |
| `test_an_experiment_applies_the_caps_without_learning` | The driver does not change during an experiment |
| `test_the_window_offers_the_proposals_and_starts_one_for_the_ai` | "AI drives it" proposes, plans and starts the experiment through core's API |

```bash
cd core && go test ./internal/experiments -run 'Driver|DifferentCars|Republished' -v
```

## 2. In the game

Follow [docs/runbooks/ghost-driver.md](../runbooks/ghost-driver.md):

1. `python tools/prepare_test_layout.py fn_cota layout_f1_2026`, install the agent, start the
   stack pointed at the game's folders.
2. Content Manager: a car, "COTA F1 2026 (Paddock test)", Practice, start from the pits.
3. Paddock window, **Run**: **Let the AI drive laps**. Watch the session appear in the
   dashboard with a "Game AI" badge and its timing grid fill.
4. **Setups**: **Diagnose this session**, then **AI drives it** on the first proposal.
5. Leave it. `Documents/Assetto Corsa/paddock/agent_status.json` and the window say which
   block and lap it is on; the experiment's laps fill in the dashboard.
6. **Judge** when the plan is driven.

## 3. The workspace

Open http://localhost:5173: the session rail on the left, the overview grid of laps by corners,
and **Telemetry**: pick a corner, tick passes to overlay them on the best one.

## What was seen, and what was not

| Claim | Evidence |
| --- | --- |
| The AI drives the player's car | Seen in game, 488 GT3 at COTA, screenshot taken inside the game |
| The agent loads a block's setup and reads it back | Spool: ARB_FRONT 3 then 2, setup hash changed, "Set and checked 1 value(s)" |
| AI laps are far more repeatable | The plan's detectable effect: 2.7 ms per corner with the AI against about 14 ms for the owner's laps |
| The AI had 20 % more grip by default | Peak lateral g 1.20 times the owner's over 16 corners; fixed by `setExtraAIGrip(0, 1)` |
| Watchdogs | A stuck car and a reset by the game were retried from the pits during experiment 2 |
| Calibration converges, formula cars stay on track | **Not verified** |
| AI-measured effects predict a human's | **Not verified** |
