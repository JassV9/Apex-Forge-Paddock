-- AI calibration (docs/adr/0016-ai-calibration.md): AC's AI is a fixed set of rules, and on the
-- car's real grip it brakes too late and runs off where it overestimates the car. What it
-- plans corners with is its tyre hint (ai.ini [TYRES] HINT): the grip it believes the car has.
-- During free laps this learns, lap by lap, how much less grip the AI must assume: a lap where
-- it went off, spun, stopped or was reset lowers the hint by STEP; two clean laps in a row mark
-- the car calibrated. The hint only ever comes down. During an experiment it is frozen, so the
-- driver never changes between two setups. Pure Lua: no CSP calls.

local aitune = {
  VERSION = 2,
  SECTIONS = 200,      -- the lap in this many sections (about 25 m on a 5 km track), for where
  STEP = 0.97,         -- a lap with an incident: the AI assumes this share of the grip it did
  MIN_HINT = 0.7,
  CLEAN_LAPS = 2,      -- this many clean laps in a row: calibrated
}

function aitune.new()
  return { hint = 1, incidents = {}, cleanLaps = 0, laps = 0, lastIncidents = 0, where = {} }
end

function aitune.section(pos)
  local s = math.floor((pos % 1) * aitune.SECTIONS)
  return math.max(0, math.min(aitune.SECTIONS - 1, s))
end

---The share of the car's AI tyre hint the AI drives with.
function aitune.hint(state) return state.hint end

-- a share outside this range is not a hint anyone pinned on purpose
aitune.PIN_MIN, aitune.PIN_MAX = 0.5, 1.5

---A pinned share as given (a run plan's ai_hint_share, a command's), or nil when it is not a
---number in PIN_MIN..PIN_MAX.
function aitune.validShare(value)
  if type(value) ~= "number" or value < aitune.PIN_MIN or value > aitune.PIN_MAX then return nil end
  return value
end

---The share the AI drives with: the pinned one when there is one, else the learned one
---(docs/adr/0028-run-context-and-ai-provenance.md). A pin never changes what was learned.
function aitune.share(state, pinned) return pinned or state.hint end

local HINT_EPSILON = 1e-6

---Why the hint the AI was given (nil: none was taken) is not the pinned one, or nil when it is
---or nothing is pinned.
function aitune.pinMismatch(base, pinned, applied)
  if not pinned then return nil end
  local want = base * pinned
  if applied and math.abs(applied - want) <= HINT_EPSILON then return nil end
  local given = applied and string.format("%.4f", applied) or "not set"
  return string.format("the AI's tyre hint is %s, the plan pins %.4f (%.4f of the car's %.4f)",
    given, want, pinned, base)
end

---Feed every frame on track while the AI drives: position, and whether something just went
---wrong (wheels off the track, a spin or stop, a reset, damage).
function aitune.observe(state, pos, incident)
  if incident then state.incidents[aitune.section(pos)] = true end
end

---Where it went wrong this lap: the first section of every run of incidents. The sections
---after, crawled through or spun in, are the result, not another mistake.
local function starts(state)
  local list = {}
  for s in pairs(state.incidents) do
    if not state.incidents[(s - 1) % aitune.SECTIONS] then list[#list + 1] = s end
  end
  table.sort(list)
  return list
end

---A lap was completed. In learning mode a lap with an incident lowers the hint; returns how
---many places the AI went wrong.
function aitune.endLap(state, learning)
  local where = starts(state)
  if learning and #where > 0 then state.hint = math.max(aitune.MIN_HINT, state.hint * aitune.STEP) end
  state.laps, state.lastIncidents, state.where = state.laps + 1, #where, where
  state.cleanLaps = #where == 0 and state.cleanLaps + 1 or 0
  state.incidents = {}
  return #where
end

function aitune.calibrated(state) return state.cleanLaps >= aitune.CLEAN_LAPS end

---What is kept between sessions.
function aitune.snapshot(state)
  return { v = aitune.VERSION, sections = aitune.SECTIONS, hint = math.floor(state.hint * 1e4 + 0.5) / 1e4,
    clean_laps = state.cleanLaps, laps = state.laps, last_incidents = state.where }
end

---A file from another version of the method starts calibration over.
function aitune.restore(saved)
  local state = aitune.new()
  if type(saved) ~= "table" or saved.v ~= aitune.VERSION or saved.sections ~= aitune.SECTIONS then
    return state
  end
  if type(saved.hint) == "number" then
    state.hint = math.max(aitune.MIN_HINT, math.min(1, saved.hint))
  end
  state.cleanLaps = tonumber(saved.clean_laps) or 0
  state.laps = tonumber(saved.laps) or 0
  return state
end

function aitune.summary(state)
  if state.laps == 0 then return "AI calibration: not started." end
  local status = aitune.calibrated(state) and "calibrated" or "learning"
  return string.format("AI calibration: %s, assumes %d %% grip, %d incident(s) last lap, %d clean lap(s).",
    status, math.floor(state.hint * 100 + 0.5), state.lastIncidents, state.cleanLaps)
end

return aitune
