-- AI calibration in the game: watches the AI on track, hands incidents to core/aitune.lua and
-- gives the AI its learned tyre hint with `physics.setAITyresHint`: the car's own hint from its
-- ai.ini times the learned share. The calibration of a car on a layout is kept in
-- Documents/Assetto Corsa/paddock/ai_tune/<car>__<track>__<layout>.json, written after a lap.
-- A pinned share (a run plan's ai_hint_share, or fixed free laps) is applied instead of the
-- learned one and never reaches that file (docs/adr/0028-run-context-and-ai-provenance.md).

local aitune = require("core.aitune")

local calibration = {
  state = aitune.new(),
  OFF_WHEELS = 3,     -- wheels outside the track that count as going off
  SLOW_KMH = 25,      -- slower than this on track for SLOW_S counts as a spin or stop
  SLOW_S = 2,
  DAMAGE_STEP = 1,
}

-- damage starts unknown: the first frame on track is its baseline, not an incident
local last = { pos = nil, reset = nil, damage = nil, slow = 0, applied = nil, wanted = nil }
local baseHint, hints = nil, nil

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

local function iniNumber(config, section, key)
  local ok, v = pcall(function() return config:get(section, key, nil) end)
  return ok and tonumber(v) or nil
end

---The car's other AI hints from its ai.ini, read once; nil where the file has none.
local function otherHints()
  if hints == nil then
    local ok, config = pcall(ac.INIConfig.carData, 0, "ai.ini")
    hints = {}
    if ok and config then
      hints.aero = iniNumber(config, "PHYSICS_HINTS", "AERO_HINT")
      hints.brake = iniNumber(config, "PEDALS", "BRAKE_HINT")
    end
  end
  return hints
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

-- the hint counts as applied only once CSP took the call; a refused call is tried again
local function apply(pinned)
  local hint = base() * aitune.share(calibration.state, pinned)
  last.wanted = hint
  if hint ~= last.applied and pcall(physics.setAITyresHint, 0, hint) then last.applied = hint end
end

---Every frame while the AI drives on track. `learning` is true in free laps only: during an
---experiment the hint is applied but never changed. With a `pinned` share the AI drives at it
---and nothing is learned, counted or written.
function calibration.update(dt, car, learning, pinned)
  apply(pinned)
  if pinned then return end
  local pos = car.splinePosition
  local hit, reset = incident(car, dt)
  aitune.observe(calibration.state, pos, hit)
  if last.pos and last.pos > 0.9 and pos < 0.1 and not reset then
    aitune.endLap(calibration.state, learning)
    save()
  end
  last.pos = pos
end

---The lap ended with a retry: an incident where the car is, and the lap is over.
function calibration.aborted(learning, pinned)
  if last.pos == nil or pinned then return end
  aitune.observe(calibration.state, last.pos, true)
  aitune.endLap(calibration.state, learning)
  save()
end

---The AI is handed the car or gives it back: the car's own hint is put back, nothing carries
---over.
function calibration.release()
  last.pos, last.reset, last.slow, last.damage, last.applied, last.wanted = nil, nil, 0, nil, nil, nil
  baseHint, hints = nil, nil  -- read afresh for every hand-over, once
  pcall(physics.setAITyresHint, 0, base())
end

---The tyre hint the AI was last given (base x share), or nil since it was handed the car.
function calibration.applied() return last.applied end

---Why the hint the AI was given is not the pinned one, or nil (also before the first try).
function calibration.hintOff(pinned)
  if last.wanted == nil then return nil end
  return aitune.pinMismatch(base(), pinned, last.applied)
end

---The AI's settings for the session line: the car's hints, the learned and the pinned share.
function calibration.settings(pinned)
  local other = otherHints()
  return {
    tyreHintBase = base(), learnedShare = aitune.hint(calibration.state), pinnedShare = pinned,
    appliedHint = base() * aitune.share(calibration.state, pinned), aeroHint = other.aero,
    brakeHint = other.brake,
  }
end

function calibration.summary() return aitune.summary(calibration.state) end
function calibration.calibrated() return aitune.calibrated(calibration.state) end

---Calibrating again checks the learned grip with fresh clean laps: on another setup or day
---the AI may not keep the car on the track where it did before.
function calibration.recount() calibration.state.cleanLaps = 0 end

return calibration
