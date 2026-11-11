-- Test driver: loads the real PaddockAgent.lua against the mocked CSP API and drives the
-- synthetic car around a line, one script.update per 60 Hz sample.
-- Adapted from Apex Forge v0 `tests/e2e.lua`.

local sim = require("sim")

local drive = {}
local M = MOCK

local DT = 1 / 60
local frames, updateSeconds, slowestFrame = 0, 0, 0

function drive.boot()
  assert(loadfile(M.appRoot .. "/PaddockAgent.lua"))()
  drive.agent = require("game.agent")
  drive.tr = sim.track(M.points, M.L)
end

function drive.profile(car)
  drive.car = car
  drive.v = sim.profile(drive.tr, car)
end

local function apply(s)
  local c = M.car
  c.splinePosition = s.d / M.L
  c.speedMs, c.speedKmh = s.v, s.v * 3.6
  c.gas, c.brake, c.steer = s.gas, s.brake, s.steer
  c.acceleration.x, c.acceleration.z = s.lat_g, s.lon_g
  c.localAngularVelocity.y = s.yaw
  local names = { [0] = "fl", "fr", "rl", "rr" }
  for i = 0, 3 do
    local w, name = c.wheels[i], names[i]
    w.slipAngle, w.slipRatio, w.load = s["sa_" .. name], s["sr_" .. name], s["load_" .. name]
  end
  c.wheelsOutside = s.out
  c.rideHeight[0], c.rideHeight[1] = s.rh_f, s.rh_r
end

local function frame()
  M.now = M.now + DT
  local started = os.clock()
  script.update(DT)
  local took = os.clock() - started
  updateSeconds = updateSeconds + took
  if took > slowestFrame then slowestFrame = took end
  frames = frames + 1
end

---@param o table sim.drive options (laps, startD, stopAt, pace, off...), plus untilPasses:
---stop as soon as the agent has recorded that many passes
function drive.run(o)
  o.car = drive.car
  o.t0 = drive.t or 0
  local t, d = sim.drive(drive.tr, drive.v, o, function(s)
    apply(s)
    frame()
    if o.untilPasses and drive.agent.passes >= o.untilPasses then o.stopAt = 0 end
  end)
  drive.t, drive.d = t + DT, d
  return t, d
end

---Frames with the car standing still (pits, pause), so pending spool writes can go out.
function drive.idle(count)
  M.car.speedMs, M.car.speedKmh = 0, 0
  for _ = 1, count do frame() end
end

function drive.pit(on)
  M.car.isInPitlane, M.car.isInPit = on, on
  drive.idle(1)
end

function drive.teleport(d)
  M.car.splinePosition = d / M.L
  M.car.resetCounter = M.car.resetCounter + 1
end

function drive.restartSession()
  for _, callback in ipairs(M.callbacks.session) do callback(0, true) end
end

function drive.release()
  for _, callback in ipairs(M.callbacks.release) do callback() end
end

function drive.timing()
  return { frames = frames, meanMs = updateSeconds / math.max(frames, 1) * 1000, maxMs = slowestFrame * 1000 }
end

return drive
