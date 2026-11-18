-- Reads the player's car once per frame into one reused sample table (no garbage per frame),
-- and the conditions of a pass when it ends. Raw values only: nothing here is scaled, averaged
-- or judged. Wheel order is the game's: 0 front left, 1 front right, 2 rear left, 3 rear right.
-- Field names are ac.StateCar's and ac.StateWheel's in CSP's Lua SDK
-- (extension/internal/lua-sdk/ac_apps/lib.lua; docs/adr/0028-run-context-and-ai-provenance.md).
--
-- Rewritten from Apex Forge v0 `game/carlink.lua`, which averaged slip per axle and scaled it
-- for the balance number that is not carried forward (docs/adr/0002-reuse-from-v0.md).

local carlink = { clock = "app" }

local sample = {
  t = 0, d = 0, v = 0, gas = 0, brake = 0, steer = 0, lat_g = 0, lon_g = 0, yaw = 0,
  sa_fl = 0, sa_fr = 0, sa_rl = 0, sa_rr = 0,
  sr_fl = 0, sr_fr = 0, sr_rl = 0, sr_rr = 0,
  load_fl = 0, load_fr = 0, load_rl = 0, load_rr = 0,
  out = 0, rh_f = 0, rh_r = 0,
  nd_fl = 0, nd_fr = 0, nd_rl = 0, nd_rr = 0,
  w_fl = 0, w_fr = 0, w_rl = 0, w_rr = 0,
  st_fl = 0, st_fr = 0, st_rl = 0, st_rr = 0,
  vx = 0, vz = 0, x = 0, z = 0, gear = 0, rpm = 0,
  kers_charge = 0, kers_input = 0, abs = 0, tc = 0, drs = 0,
}
carlink.sample = sample

-- physics time that stands still this long while the game runs is not a clock
carlink.FROZEN_S = 0.5
local stamp = { ms = nil, at = 0 }

---Which clock stamps the samples: CSP's physics time (`car.timestamp`, ms) when the car has
---one at start-up, else the app's own clock. Physics time does not jitter with the frame rate.
---@param car ac.StateCar
function carlink.chooseClock(car)
  carlink.clock = type(car.timestamp) == "number" and car.timestamp > 0 and "physics" or "app"
  stamp.ms, stamp.at = nil, 0
  return carlink.clock
end

-- seconds of the chosen clock; a physics time that stops moving hands over to the app clock
-- for good (said on every pass line from then on)
local function now(car, t)
  if carlink.clock ~= "physics" then return t end
  local ms = car.timestamp
  if ms ~= stamp.ms then
    stamp.ms, stamp.at = ms, t
  elseif t - stamp.at > carlink.FROZEN_S then
    carlink.clock = "app"
    return t
  end
  return ms / 1000
end

local function flag(value) return value and 1 or 0 end

-- the channels of docs/adr/0028: normalised slip, wheel speed, suspension travel, body velocity,
-- world position, gear, rpm, and whether ABS, TC and DRS act
local function readExtra(s, car, fl, fr, rl, rr)
  s.nd_fl, s.nd_fr, s.nd_rl, s.nd_rr = fl.ndSlip or 0, fr.ndSlip or 0, rl.ndSlip or 0, rr.ndSlip or 0
  s.w_fl, s.w_fr, s.w_rl, s.w_rr = fl.angularSpeed or 0, fr.angularSpeed or 0, rl.angularSpeed or 0,
    rr.angularSpeed or 0
  s.st_fl, s.st_fr, s.st_rl, s.st_rr = fl.suspensionTravel or 0, fr.suspensionTravel or 0,
    rl.suspensionTravel or 0, rr.suspensionTravel or 0
  local velocity, position = car.localVelocity, car.position
  s.vx, s.vz = velocity and velocity.x or 0, velocity and velocity.z or 0
  s.x, s.z = position and position.x or 0, position and position.z or 0
  s.gear, s.rpm = car.gear or 0, car.rpm or 0
  s.abs, s.tc, s.drs = flag(car.absInAction), flag(car.tractionControlInAction), flag(car.drsActive)
end

---@param car ac.StateCar
---@param trackLength number metres
---@param t number app clock, seconds (used when the car has no physics time)
function carlink.read(car, trackLength, t)
  local s = sample
  local wheels = car.wheels
  local fl, fr, rl, rr = wheels[0], wheels[1], wheels[2], wheels[3]
  s.t = now(car, t)
  s.d = car.splinePosition * trackLength
  s.v = car.speedMs
  s.gas, s.brake, s.steer = car.gas, car.brake, car.steer
  s.lat_g, s.lon_g = car.acceleration.x, car.acceleration.z
  s.yaw = car.localAngularVelocity.y
  s.sa_fl, s.sa_fr, s.sa_rl, s.sa_rr = fl.slipAngle, fr.slipAngle, rl.slipAngle, rr.slipAngle
  s.sr_fl, s.sr_fr, s.sr_rl, s.sr_rr = fl.slipRatio, fr.slipRatio, rl.slipRatio, rr.slipRatio
  s.load_fl, s.load_fr, s.load_rl, s.load_rr = fl.load, fr.load, rl.load, rr.load
  s.out = car.wheelsOutside
  s.rh_f, s.rh_r = car.rideHeight[0], car.rideHeight[1]
  readExtra(s, car, fl, fr, rl, rr)
  -- KERS/ERS battery charge and deployment input, both 0..1 (docs/adr/0024-ers-state-per-pass.md)
  s.kers_charge, s.kers_input = car.kersCharge or 0, car.kersInput or 0
  return s
end

---True when the car has a KERS/ERS battery: only then does a pass line carry its state.
---@param car ac.StateCar
function carlink.hasErs(car)
  return car.kersPresent == true
end

---The largest of the car's damage zones (AC's units).
---@param car ac.StateCar
function carlink.damage(car)
  return math.max(car.damage[0], car.damage[1], car.damage[2], car.damage[3])
end

local function perWheel(car, field)
  local wheels = car.wheels
  return { wheels[0][field] or 0, wheels[1][field] or 0, wheels[2][field] or 0, wheels[3][field] or 0 }
end

---Car and track state for one pass record. Called once per pass, so it may allocate.
---@param car ac.StateCar
---@param sim ac.StateSim
function carlink.conditions(car, sim)
  return {
    fuel = car.fuel or 0,
    core = perWheel(car, "tyreCoreTemperature"),
    surface = perWheel(car, "tyreMiddleTemperature"),
    pressure = perWheel(car, "tyrePressure"),
    wear = perWheel(car, "tyreWear"),
    grip = sim.roadGrip or 0,
    air = sim.ambientTemperature or 0,
    road = sim.roadTemperature or 0,
  }
end

return carlink
