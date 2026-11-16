-- AI calibration in the game: watches the AI on track, hands incidents to core/aitune.lua and
-- gives the AI its learned tyre hint with `physics.setAITyresHint`: the car's own hint from its
-- ai.ini times the learned share. The calibration of a car on a layout is kept in
-- Documents/Assetto Corsa/paddock/ai_tune/<car>__<track>__<layout>.json, written after a lap.

local aitune = require("core.aitune")

local calibration = {
  state = aitune.new(),
  OFF_WHEELS = 3,     -- wheels outside the track that count as going off
  SLOW_KMH = 25,      -- slower than this on track for SLOW_S counts as a spin or stop
  SLOW_S = 2,
  DAMAGE_STEP = 1,
}

-- damage starts unknown: the first frame on track is its baseline, not an incident
local last = { pos = nil, reset = nil, damage = nil, slow = 0, applied = nil }
local baseHint = nil

local function safe(text) return (tostring(text or ""):gsub("[^%w%._%-]+", "_")) end

local function path()
  return ac.getFolder(ac.FolderID.ACDocuments) .. "\\paddock\\ai_tune\\"
    .. safe(ac.getCarID(0)) .. "__" .. safe(ac.getTrackID()) .. "__" .. safe(ac.getTrackLayout()) .. ".json"
end

---The car's own tyre hint, read once.
local function base()
  if baseHint == nil then
    local ok, v = pcall(function() return ac.INIConfig.carData(0, "ai.ini"):get("TYRES", "HINT", 1) end)
    baseHint = ok and tonumber(v) or 1
  end
  return baseHint
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

local function apply()
  local hint = base() * aitune.hint(calibration.state)
  if hint ~= last.applied then
    last.applied = hint
    physics.setAITyresHint(0, hint)
  end
end

---Every frame while the AI drives on track. `learning` is true in free laps only: during an
---experiment the hint is applied but never changed.
function calibration.update(dt, car, learning)
  apply()
  local pos = car.splinePosition
  local hit, reset = incident(car, dt)
  aitune.observe(calibration.state, pos, hit)
  if last.pos and last.pos > 0.9 and pos < 0.1 and not reset then
    aitune.endLap(calibration.state, learning)
    save()
  end
  last.pos = pos
end

---The AI is handed the car or gives it back: the car's own hint is put back, nothing carries
---over.
function calibration.release()
  last.pos, last.reset, last.slow, last.damage, last.applied = nil, nil, 0, nil, nil
  pcall(physics.setAITyresHint, 0, base())
end

function calibration.summary() return aitune.summary(calibration.state) end

return calibration
