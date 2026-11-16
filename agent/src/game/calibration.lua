-- AI calibration in the game: watches the AI on track, hands incidents to core/aitune.lua and
-- applies its speed caps section by section with `physics.setAITopSpeed`. The caps of a car
-- on a layout are kept in Documents/Assetto Corsa/paddock/ai_tune/<car>__<layout>.json, written
-- when a lap changed them.

local aitune = require("core.aitune")

local calibration = {
  state = aitune.new(),
  OFF_WHEELS = 3,     -- wheels outside the track that count as going off
  SLOW_KMH = 25,      -- slower than this on track for SLOW_S counts as a spin or stop
  SLOW_S = 2,
  DAMAGE_STEP = 1,
}

-- damage starts unknown: the first frame on track is its baseline, not an incident
local last = { pos = nil, section = -1, reset = nil, damage = nil, slow = 0 }

local function safe(text) return (tostring(text or ""):gsub("[^%w%._%-]+", "_")) end

local function path()
  return ac.getFolder(ac.FolderID.ACDocuments) .. "\\paddock\\ai_tune\\"
    .. safe(ac.getCarID(0)) .. "__" .. safe(ac.getTrackID()) .. "__" .. safe(ac.getTrackLayout()) .. ".json"
end

function calibration.load()
  local ok, saved = pcall(JSON.parse, io.load(path(), "") or "")
  calibration.state = aitune.restore(ok and saved or nil)
end

local function save()
  io.createDir(ac.getFolder(ac.FolderID.ACDocuments) .. "\\paddock\\ai_tune")
  io.saveAsync(path(), JSON.stringify(aitune.snapshot(calibration.state)), nil, true)
end

local function incident(car, dt)
  local damage = math.max(car.damage[0], car.damage[1], car.damage[2], car.damage[3])
  local reset = last.reset ~= nil and car.resetCounter ~= last.reset
  last.slow = car.speedKmh < calibration.SLOW_KMH and last.slow + dt or 0
  local hit = car.wheelsOutside >= calibration.OFF_WHEELS or reset or last.slow >= calibration.SLOW_S
    or damage > (last.damage or damage) + calibration.DAMAGE_STEP
  last.reset, last.damage = car.resetCounter, damage
  return hit, reset
end

---Every frame while the AI drives on track. `learning` is true in free laps only: during an
---experiment the caps are applied but never changed.
function calibration.update(dt, car, learning)
  local pos = car.splinePosition
  local hit, reset = incident(car, dt)
  aitune.observe(calibration.state, pos, car.speedKmh, hit)
  local crossed = last.pos and last.pos > 0.9 and pos < 0.1 and not reset
  if crossed then
    local before = JSON.stringify(aitune.snapshot(calibration.state).caps)
    aitune.endLap(calibration.state, learning)
    if JSON.stringify(aitune.snapshot(calibration.state).caps) ~= before or learning then save() end
  end
  last.pos = pos
  local section = aitune.section(pos)
  if section ~= last.section then
    last.section = section
    physics.setAITopSpeed(0, aitune.capAt(calibration.state, pos) or 1e9)
  end
end

---The AI is handed the car or gives it back: no cap may outlive the run, nothing carries over.
function calibration.release()
  last.pos, last.section, last.reset, last.slow, last.damage = nil, -1, nil, 0, nil
  pcall(physics.setAITopSpeed, 0, 1e9)
end

function calibration.summary() return aitune.summary(calibration.state) end

return calibration
