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
-- says so (phase failed). A block whose setup is not on the car after loading or leaving the
-- pits stops the run at once, naming what differs: its laps would count for nothing, and a
-- reload would not keep a value the car does not hold. `stop` hands the car back.
--
-- Without a plan, `lap` starts a free run: the AI laps the car on the setup it has, as block
-- 0, until it is stopped or a plan arrives. Its passes are the AI's baseline on this car.
--
-- A game that starts with an experiment for the AI open on the car is the driver's, not the
-- AI's (docs/adr/0021-human-keeps-the-car.md): `hold` keeps the AI from taking the car, for
-- that experiment and for any that follows, until `resume`, `lap` or `calibrate`.
--
-- Run context (docs/adr/0028-run-context-and-ai-provenance.md): the fuel is read back after
-- every refuel and a block that does not hold it stops the run; a plan that pins the AI's tyre
-- hint stops the run when the hint the AI was given is another; free laps can run at a fixed
-- share without learning.

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
  FUEL_CHECK_S = 1,   -- the fuel is read back this long after the block was handed to the AI
  FUEL_TOLERANCE_L = 0.5,
  FUEL_RESETS = 3,    -- a read-back that is off sets the fuel again this many times before stopping
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

-- the first block's fuel, never less than MIN_FUEL_L, never more than the tank holds
local function fuelTarget(o)
  local liters = math.max(o.fuel or 0, ghost.MIN_FUEL_L)
  if o.maxFuel and o.maxFuel > 0 then liters = math.min(liters, o.maxFuel) end
  return liters
end

local function fromOff(s, o)
  if not wanted(s, o) then return {} end
  if s.held then return holdBack(s, o) end
  if not o.allowed then
    s.message = "The AI cannot drive here: this track does not give apps physics access."
    return {}
  end
  s.fuel, s.retries = fuelTarget(o), 0
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

local function fail(s, why)
  s.phase = "failed"
  s.message = "AI driver stopped: " .. why .. "."
  return { act("autopilot_off") }
end

local function retry(s, o, why)
  -- free laps have no end: three retries without a lap in between stop them, not three ever
  if s.block == 0 and o.lapsDone ~= s.retryLap then s.retries = 0 end
  s.retryLap = o.lapsDone
  s.retries = s.retries + 1
  if s.retries > ghost.MAX_RETRIES then
    return fail(s, string.format("%s %d times in %s", why, s.retries, name(s.block)))
  end
  return goToPits(s, o.block, string.format("AI driver: %s, retrying %s.", why, name(o.block)))
end

-- the fuel read back FUEL_CHECK_S after the block began: a refuel that did not take (seen:
-- experiment 15 drove block 1 on 24 L, the others on 48 L) would make the arms unequal. The
-- setup's own [FUEL] VALUE can land after the set (seen: 26 L at block 9 of experiment 20,
-- the first block after a relaunch), so the fuel is set again during the out-lap, up to
-- FUEL_RESETS times, before the block stops
local function checkFuel(s, o)
  if not s.fuelCheck then return nil end
  s.fuelCheck = s.fuelCheck - o.dt
  if s.fuelCheck > 0 then return nil end
  s.fuelCheck = nil
  if o.fuel == nil or math.abs(o.fuel - s.fuel) <= ghost.FUEL_TOLERANCE_L then return nil end
  s.fuelResets = (s.fuelResets or 0) + 1
  if s.fuelResets <= ghost.FUEL_RESETS then
    s.fuelCheck = ghost.FUEL_CHECK_S
    return { act("set_fuel", { liters = s.fuel }) }
  end
  return fail(s, string.format("the car has %.1f L after refuelling for %s, the plan wants %.1f L"
    .. " (set %d times)", o.fuel, name(s.block), s.fuel, s.fuelResets))
end

-- why the block cannot count: its setup is not on the car or the AI's hint is not the plan's
local function invalid(s, o)
  if s.block ~= 0 and o.offCar then return o.offCar end
  return o.hintOff
end

-- the block stops when it cannot count; its fuel is set again while that can still help
local function offPlan(s, o)
  local why = invalid(s, o)
  if why then return fail(s, why) end
  return checkFuel(s, o)
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
  local off = offPlan(s, o)
  if off then return off end
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
---has counted in this block), calibrated (the AI calibration has its clean laps), offCar (why
---the block's setup is not on the car, or nil), maxFuel (the tank, litres), hintOff (why the
---AI's tyre hint is not the one the plan pins, or nil).
---@return table[] actions, in order: autopilot_on/off, teleport_pits, repair, repair_body,
---set_fuel, load, start
function ghost.step(s, o)
  if o.planId ~= s.planId then  -- a new experiment: an earlier stop or choice does not carry over
    s.planId, s.halted, s.override, s.free = o.planId, false, false, false
  end
  -- free laps are the car on its own setup, block 0, even with a plan open for a human
  if s.free and (o.block == nil or not o.driverAi) then o.block = 0 end
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

---The result of the `load` action; `offCar` says why the setup read back afresh is not the
---block's (nil when it is).
function ghost.loaded(s, ok, message, offCar)
  if offCar then return fail(s, offCar) end
  if not ok then return fail(s, "the setup did not load (" .. tostring(message) .. ")") end
  s.phase, s.stuck, s.damage, s.sinceLap, s.fuelCheck = "driving", 0, nil, 0, ghost.FUEL_CHECK_S
  s.fuelResets = 0
  s.resets, s.lapsDone = nil, nil  -- the teleport and the start count as resets; begin afresh
  s.message = "AI driver: driving " .. name(s.block) .. "."
  -- the fuel again: resetCarState (the repair) takes effect a step later and can put back the
  -- session's starting fuel over the first set_fuel (seen in game: 26 L for a 50 L block)
  return { act("set_fuel", { liters = s.fuel }), act("autopilot_on"), act("start") }
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

---Let the AI lap the car on its current setup until stopped (when there is no plan). It learns
---its tyre hint, unless `fixed`: then it drives at `share` of the car's hint (the learned share
---when nil) and nothing it does changes the calibration.
---@param fixed boolean|nil
---@param share number|nil
function ghost.lap(s, fixed, share)
  s.free, s.halted, s.held, s.retries, s.untilCalibrated = true, false, false, 0, false
  s.learn, s.freeShare = not fixed, fixed and share or nil
  if s.phase == "failed" then s.phase = "off" end
end

---True while free laps teach the AI its tyre hint.
function ghost.learning(s) return s.block == 0 and s.learn ~= false end

---The share of the car's tyre hint pinned for what the AI drives now, or nil (the learned share
---applies): a plan's pin in its blocks, the fixed share in free laps.
function ghost.pinnedShare(s, planShare)
  if s.block == 0 then return s.freeShare end
  return planShare
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
