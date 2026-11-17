-- Ghost Driver in the game: performs what core/ghost.lua decides with CSP's physics API.
-- `physics.*` works only on a track that gives Lua apps physics access (tools/
-- prepare_test_layout.py makes such a copy of a layout). Elsewhere the agent says so and the
-- human drives. Never used online: AC's autopilot and teleports are offline features.

local calibration = require("game.calibration")
local core = require("core.ghost")
local plan = require("game.plan")

local ghost = { state = core.new(), KEEP_OUT_S = 2 }

-- In practice AC's AI drives a stint and then goes back to the pits by itself. It must not:
-- a pit stop changes fuel and tyres in the middle of a block. CSP's block on pitting wears
-- off, so it is set again every KEEP_OUT_S while the AI drives. AC gives AI cars 120 % grip by
-- default; measured on the player's car the AI pulled 1.20 times the human's lateral g, so
-- the grip is put back to the car's own, or every effect would be measured on another car.
local function keepOut()
  physics.setAILapsToComplete(0, 1000)
  physics.setAIPitStopRequest(0, false)
  physics.setExtraAIGrip(0, 1)
end
local nextKeepOut = 0

local function allowed()
  if not physics or not physics.allowed then return false end
  local ok, v = pcall(physics.allowed)
  return ok and v == true
end

local perform

local handlers = {
  autopilot_on = function()
    physics.setCarAutopilot(true, false)
    keepOut()
  end,
  autopilot_off = function()
    calibration.release()
    physics.setCarAutopilot(false, false)
  end,
  teleport_pits = function() physics.teleportCarTo(0, ac.SpawnSet.Pits) end,
  repair = function() physics.resetCarState(0, 1) end,
  -- body only, without resetCarState's fuel reset: the car keeps driving
  repair_body = function() physics.setCarBodyDamage(0, vec4(0, 0, 0, 0)) end,
  set_fuel = function(a) if a.liters then physics.setCarFuel(0, a.liters) end end,
  start = function() if ac.tryToStart then ac.tryToStart(true) end end,
  load = function()
    if ghost.state.block == 0 then return perform(core.loaded(ghost.state, true, "")) end
    local ok = plan.loadBlock()
    perform(core.loaded(ghost.state, ok, plan.message))
  end,
}

perform = function(actions)
  for _, a in ipairs(actions) do
    local ok, err = pcall(handlers[a.name], a)
    if not ok then ac.log("Paddock agent: ghost action " .. a.name .. " failed: " .. tostring(err)) end
  end
end

---Every frame, before the car is sampled.
local calibrationLoaded = false

function ghost.update(dt, car)
  if not calibrationLoaded then
    calibrationLoaded = true
    pcall(calibration.load)
  end
  nextKeepOut = nextKeepOut - dt
  if ghost.state.phase == "driving" and nextKeepOut <= 0 then
    nextKeepOut = ghost.KEEP_OUT_S
    pcall(keepOut)
  end
  local retries = ghost.state.retries
  local lapsDone = plan.lapsDone()  -- the plan's count; a free run counts the car's laps
  if plan.blockOrder() == nil or (ghost.state.free and plan.driver() ~= "ai") then
    lapsDone = car.lapCount
  end
  local actions = core.step(ghost.state, {
    dt = dt, planId = plan.experimentId(), driverAi = plan.driver() == "ai", allowed = allowed(),
    block = plan.blockOrder(), parked = car.isInPit and car.speedKmh < 1,
    onTrack = not car.isInPitlane, speedKmh = car.speedKmh, ai = car.isAIControlled, fuel = car.fuel,
    damage = math.max(car.damage[0], car.damage[1], car.damage[2], car.damage[3]),
    resets = car.resetCounter, lapsDone = lapsDone, calibrated = calibration.calibrated(),
  })
  -- a block retried (reset by the game, stuck, damaged) is the clearest incident of all: the
  -- lap it ends is counted before the car leaves the track
  if ghost.state.retries > retries then calibration.aborted(ghost.state.block == 0) end
  perform(actions)
  -- free laps teach the AI where it needs a lower speed; an experiment only applies it
  if ghost.state.phase == "driving" and not car.isInPitlane then
    calibration.update(dt, car, ghost.state.block == 0)
  end
end

function ghost.active() return core.active(ghost.state) end
function ghost.phase() return ghost.state.phase end
function ghost.message() return ghost.state.message end
function ghost.calibration() return calibration.summary() end
function ghost.allowed() return allowed() end
function ghost.stop() perform(core.stop(ghost.state)) end
function ghost.resume() core.resume(ghost.state) end
function ghost.hold() core.hold(ghost.state) end
function ghost.held() return ghost.state.held end
---True while the AI laps the car on its own setup (block 0): no experiment owns those laps.
function ghost.freeRun() return core.active(ghost.state) and ghost.state.block == 0 end
function ghost.lap() core.lap(ghost.state) end
function ghost.calibrate()
  calibration.recount()
  core.calibrate(ghost.state)
end

-- Commands from outside the game (outbox/agent_command.json: {"id": n, "at": unix seconds,
-- "command": "ai_laps" | "calibrate" | "stop"}), read every COMMAND_POLL_S. Each id is acted on once,
-- and only within COMMAND_TTL_S of `at`, so a reloaded agent never replays an old command.
ghost.COMMAND_POLL_S, ghost.COMMAND_TTL_S = 2, 60
local nextCommand, lastCommand = 0, nil
function ghost.pollCommands(now)
  if now < nextCommand then return end
  nextCommand = now + ghost.COMMAND_POLL_S
  local path = ac.getFolder(ac.FolderID.ACDocuments) .. "\\paddock\\outbox\\agent_command.json"
  local ok, cmd = pcall(JSON.parse, io.load(path, "") or "")
  if not ok or type(cmd) ~= "table" or cmd.id == nil or cmd.id == lastCommand then return end
  lastCommand = cmd.id
  if type(cmd.at) ~= "number" or math.abs(os.time() - cmd.at) > ghost.COMMAND_TTL_S then return end
  local run = { ai_laps = ghost.lap, calibrate = ghost.calibrate, stop = ghost.stop }
  if run[cmd.command] then run[cmd.command]() end
end
function ghost.reset() core.reset(ghost.state) end

return ghost
