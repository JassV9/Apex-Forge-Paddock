-- Ghost Driver, level 2 (docs/adr/0014-ghost-driver.md): AC's own AI drives the run plan's
-- blocks on the player's car. For every block the car is put in its pit box, repaired, given
-- the same fuel as the first block, gets the block's setup (read back like a human load), and
-- is handed to the AI. The run plan counts the laps and tags the passes exactly as for a human.
--
-- This file only decides; game/ghost.lua performs the actions. Pure Lua: no CSP calls.
--
-- Phases: off -> to_pits -> (load) -> driving -> to_pits (next block) ... -> off when the plan
-- is complete. A car that stands still (on track, or parked by the AI's own pit stop) or takes
-- damage is sent back to the pits to retry its block; after MAX_RETRIES the run stops and
-- says so (phase failed). `stop` hands the car back.
--
-- Without a plan, `lap` starts a free run: the AI laps the car on the setup it has, as block
-- 0, until it is stopped or a plan arrives. Its passes are the AI's baseline on this car.
--
-- A game that starts with an experiment for the AI open on the car is the driver's, not the
-- AI's (docs/adr/0021-human-keeps-the-car.md): `hold` keeps the AI from taking the car, for
-- that experiment and for any that follows, until `resume`, `lap` or `calibrate`.

local ghost = {
  STUCK_KMH = 15,     -- slower than this on track counts as stuck
  STUCK_S = 20,       -- for this long
  DAMAGE_STEP = 5,    -- a rise in any damage zone above this (AC's units, km/h of impact)
  LAP_TIMEOUT_S = 600, -- no lap counted for this long: the block is retried
  SETTLE_S = 2,       -- parked this long after a teleport before the setup is loaded
  TELEPORT_RETRY_S = 10,
  MAX_RETRIES = 3,
  MIN_FUEL_L = 30,    -- every block starts with the first block's fuel, but never less than this
  RESERVE_L = 4,      -- free laps go in to refuel below this; a dry car heads for the pits
}

function ghost.new()
  return { phase = "off", block = nil, timer = 0, stuck = 0, retries = 0, fuel = nil,
    halted = false, override = false, free = false, held = false, message = "" }
end

local function act(name, extra)
  local a = extra or {}
  a.name = name
  return a
end

local function name(block) return block == 0 and "free laps" or ("block " .. block) end

local function goToPits(s, block, message)
  s.phase, s.block, s.timer, s.stuck, s.sinceLap = "to_pits", block, 0, 0, 0
  s.message = message
  return { act("autopilot_off"), act("teleport_pits") }
end

---True while the AI has, or is about to get, the car: passes are recorded and laps counted.
function ghost.active(s) return s.phase == "to_pits" or s.phase == "driving" end

---Whether the run should be driven by the AI: the plan asks for it, or the driver chose it
---in the window for this plan.
local function wanted(s, o)
  return (o.driverAi or s.override or s.free) and o.block ~= nil and not s.halted
end

-- the AI is wanted but the driver has the car: say so once per experiment, do nothing
local function holdBack(s, o)
  if s.heldFor ~= o.planId then
    s.heldFor = o.planId
    s.message = string.format("Experiment %s is for the AI to drive. You have the car: the AI does not"
      .. " start by itself and your laps are not counted in it.", tostring(o.planId))
  end
  return {}
end

local function fromOff(s, o)
  if not wanted(s, o) then return {} end
  if s.held then return holdBack(s, o) end
  if not o.allowed then
    s.message = "The AI cannot drive here: this track does not give apps physics access."
    return {}
  end
  s.fuel, s.retries = math.max(o.fuel or 0, ghost.MIN_FUEL_L), 0
  return goToPits(s, o.block, "AI driver: to the pits for " .. name(o.block) .. ".")
end

local function inPits(s, o)
  s.timer = s.timer + o.dt
  if o.parked and s.timer >= ghost.SETTLE_S then
    s.phase = "loading"
    return { act("repair"), act("set_fuel", { liters = s.fuel }), act("load") }
  end
  if not o.parked and s.timer >= ghost.TELEPORT_RETRY_S then
    s.timer = 0
    return { act("teleport_pits") }
  end
  return {}
end

local function retry(s, o, why)
  -- free laps have no end: three retries without a lap in between stop them, not three ever
  if s.block == 0 and o.lapsDone ~= s.retryLap then s.retries = 0 end
  s.retryLap = o.lapsDone
  s.retries = s.retries + 1
  if s.retries > ghost.MAX_RETRIES then
    s.phase = "failed"
    s.message = string.format("AI driver stopped: %s %d times in %s.", why, s.retries, name(s.block))
    return { act("autopilot_off") }
  end
  return goToPits(s, o.block, string.format("AI driver: %s, retrying %s.", why, name(o.block)))
end

local function whileDriving(s, o)
  if not wanted(s, o) then
    s.phase = "off"
    s.message = o.block and "AI driver: stopped." or "AI driver: run plan complete."
    return { act("autopilot_off"), act("teleport_pits") }
  end
  if o.block ~= s.block then
    s.retries = 0
    return goToPits(s, o.block, "AI driver: block done, to the pits for " .. name(o.block) .. ".")
  end
  if s.block == 0 and s.untilCalibrated and o.calibrated then
    s.phase, s.free, s.untilCalibrated = "off", false, false
    s.message = "AI calibrated on this car: the car is yours, or start an experiment."
    return { act("autopilot_off"), act("teleport_pits") }
  end
  if s.block == 0 and o.fuel and o.fuel < ghost.RESERVE_L then
    return goToPits(s, 0, "AI driver: refuelling for free laps.")
  end
  if o.speedKmh < ghost.STUCK_KMH then s.stuck = s.stuck + o.dt else s.stuck = 0 end
  if s.stuck >= ghost.STUCK_S then return retry(s, o, "stuck") end
  -- the damage a block starts with is its baseline (a repair may show a frame late); a
  -- repair lowers it
  local damage = o.damage or 0
  s.damage = math.min(s.damage or damage, damage)
  -- brushing the pit wall on the way out is not a crash, and nothing is measured yet: any of
  -- it is repaired, so every block reaches the track on an undamaged car
  if not o.onTrack and damage > s.damage then return { act("repair_body") } end
  if damage > s.damage + ghost.DAMAGE_STEP then return retry(s, o, "damaged") end
  -- AC puts a car it thinks is lost back on track, often past the line, so the lap never ends
  if o.resets ~= nil and s.resets ~= nil and o.resets ~= s.resets then return retry(s, o, "reset by the game") end
  s.resets = o.resets
  if o.lapsDone ~= s.lapsDone then s.lapsDone, s.sinceLap = o.lapsDone, 0 end
  s.sinceLap = (s.sinceLap or 0) + o.dt
  if s.block ~= 0 and s.sinceLap >= ghost.LAP_TIMEOUT_S then return retry(s, o, "no lap counted") end
  if not o.ai then return { act("autopilot_on") } end
  return {}
end

---One frame. `o`: dt, planId (experiment of the loaded plan, or nil), driverAi, allowed
---(physics access), block (order being driven, or nil),
---parked (in the pit box and still), onTrack, speedKmh, ai (car is AI controlled), fuel,
---damage (the largest damage zone), resets (the car's reset counter), lapsDone (laps the plan
---has counted in this block), calibrated (the AI calibration has its clean laps).
---@return table[] actions, in order: autopilot_on/off, teleport_pits, repair, repair_body,
---set_fuel, load, start
function ghost.step(s, o)
  if o.planId ~= s.planId then  -- a new experiment: an earlier stop or choice does not carry over
    s.planId, s.halted, s.override, s.free = o.planId, false, false, false
  end
  if o.block == nil and s.free then o.block = 0 end
  if s.phase == "off" then return fromOff(s, o) end
  if s.phase == "to_pits" then
    if not wanted(s, o) then
      s.phase = "off"
      return { act("autopilot_off") }
    end
    return inPits(s, o)
  end
  if s.phase == "driving" then return whileDriving(s, o) end
  return {}  -- loading waits for `loaded`; failed waits for `resume`
end

---The result of the `load` action.
function ghost.loaded(s, ok, message)
  if not ok then
    s.phase = "failed"
    s.message = "AI driver stopped: the setup did not load (" .. tostring(message) .. ")."
    return { act("autopilot_off") }
  end
  s.phase, s.stuck, s.damage, s.sinceLap = "driving", 0, nil, 0
  s.resets, s.lapsDone = nil, nil  -- the teleport and the start count as resets; begin afresh
  s.message = "AI driver: driving " .. name(s.block) .. "."
  return { act("autopilot_on"), act("start") }
end

---The driver takes the car back. The AI does not start again until `resume`.
function ghost.stop(s)
  local was = s.phase
  s.halted, s.override, s.free, s.phase = true, false, false, "off"
  s.message = "AI driver: stopped, the car is yours."
  if was == "off" then return {} end
  return { act("autopilot_off") }
end

---Keep the AI from taking the car by itself: the game started with an AI experiment open, so
---the person at the wheel decides when the AI drives.
function ghost.hold(s) s.held = true end

---Let the AI drive (again), also a plan that asked for a human driver.
function ghost.resume(s)
  s.halted, s.override, s.held, s.retries = false, true, false, 0
  if s.phase == "failed" then s.phase = "off" end
end

---Let the AI lap the car on its current setup until stopped (when there is no plan).
function ghost.lap(s)
  s.free, s.halted, s.held, s.retries, s.untilCalibrated = true, false, false, 0, false
  if s.phase == "failed" then s.phase = "off" end
end

---Free laps that end by themselves once the calibration says calibrated (`calibrated` in
---the observation), with the car parked in its pit box.
function ghost.calibrate(s)
  ghost.lap(s)
  s.untilCalibrated = true
end

---The session restarted: whatever the AI was doing starts over from the pits.
function ghost.reset(s)
  if s.phase ~= "off" and s.phase ~= "failed" then s.phase = "off" end
end

return ghost
