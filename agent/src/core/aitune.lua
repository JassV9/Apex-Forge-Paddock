-- AI calibration (docs/adr/0016-ai-calibration.md): AC's AI is a fixed set of rules, and on real
-- grip it runs wide or spins where it overestimates the car. During free laps this learns,
-- section by section, where the AI needs a lower top speed: a section where it went off,
-- spun, stopped or was reset gets a speed cap on its approach, lowered again if it still goes
-- wrong. The caps only ever come down, and learning stops once laps are clean. During an
-- experiment the caps are frozen, so the driver never changes between two setups.
-- Pure Lua: no CSP calls.

local aitune = {
  VERSION = 1,
  SECTIONS = 200,         -- the lap in this many sections (about 25 m on a 5 km track)
  LOOKBACK = 6,           -- sections before an incident that share its cap (the braking zone)
  CAP_FACTOR = 0.93,      -- a first cap: this share of the speed the AI carried there
  STEP = 0.95,            -- an existing cap is lowered by this factor
  MIN_KMH = 40,
  CLEAN_LAPS = 2,         -- this many clean laps in a row: calibrated
}

function aitune.new()
  return { caps = {}, entry = {}, incidents = {}, cleanLaps = 0, laps = 0, lastIncidents = 0 }
end

function aitune.section(pos)
  local s = math.floor((pos % 1) * aitune.SECTIONS)
  return math.max(0, math.min(aitune.SECTIONS - 1, s))
end

---The speed cap at a position along the lap (0..1), or nil where the AI may go as it likes.
function aitune.capAt(state, pos) return state.caps[aitune.section(pos)] end

---Feed every frame on track while the AI drives: position, speed, and whether something just
---went wrong (wheels off the track, a spin or stop, a reset, damage).
function aitune.observe(state, pos, speedKmh, incident)
  local s = aitune.section(pos)
  if state.entry[s] == nil then state.entry[s] = speedKmh end
  if incident then state.incidents[s] = true end
end

local function lower(state, s, carried)
  local current = state.caps[s]
  -- a section the AI did not drive this lap (an out-lap joins past the line) has no speed to
  -- cap from
  if current == nil and carried == nil then return end
  local proposed = current and current * aitune.STEP or carried * aitune.CAP_FACTOR
  state.caps[s] = math.max(aitune.MIN_KMH, current and math.min(current, proposed) or proposed)
end

---A lap was completed. In learning mode the incidents of the lap become caps; returns how
---many sections had an incident.
function aitune.endLap(state, learning)
  local count = 0
  for s in pairs(state.incidents) do
    count = count + 1
    if learning then
      for back = 0, aitune.LOOKBACK do
        local t = (s - back) % aitune.SECTIONS
        lower(state, t, state.entry[t])
      end
    end
  end
  state.laps, state.lastIncidents = state.laps + 1, count
  state.cleanLaps = count == 0 and state.cleanLaps + 1 or 0
  state.entry, state.incidents = {}, {}
  return count
end

function aitune.calibrated(state) return state.cleanLaps >= aitune.CLEAN_LAPS end

---What is kept between sessions: the caps, as a list of {section, kmh}.
function aitune.snapshot(state)
  local caps = {}
  for s, v in pairs(state.caps) do caps[#caps + 1] = { s, math.floor(v * 10 + 0.5) / 10 } end
  table.sort(caps, function(a, b) return a[1] < b[1] end)
  return { v = aitune.VERSION, sections = aitune.SECTIONS, caps = caps,
    clean_laps = state.cleanLaps, laps = state.laps }
end

function aitune.restore(saved)
  local state = aitune.new()
  if type(saved) ~= "table" or saved.v ~= aitune.VERSION or saved.sections ~= aitune.SECTIONS then
    return state
  end
  for _, pair in ipairs(saved.caps or {}) do state.caps[pair[1]] = pair[2] end
  state.cleanLaps, state.laps = saved.clean_laps or 0, saved.laps or 0
  return state
end

---One line for the window.
function aitune.summary(state)
  local n = 0
  for _ in pairs(state.caps) do n = n + 1 end
  if state.laps == 0 and n == 0 then return "AI calibration: not started." end
  local status = aitune.calibrated(state) and "calibrated" or "learning"
  return string.format("AI calibration: %s, %d speed caps, %d incident(s) last lap, %d clean lap(s).",
    status, n, state.lastIncidents, state.cleanLaps)
end

return aitune
