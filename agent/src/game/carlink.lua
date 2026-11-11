-- Reads the player's car once per frame into one reused sample table (no garbage per frame),
-- and the conditions of a pass when it ends. Raw values only: nothing here is scaled, averaged
-- or judged. Wheel order is the game's: 0 front left, 1 front right, 2 rear left, 3 rear right.
--
-- Rewritten from Apex Forge v0 `game/carlink.lua`, which averaged slip per axle and scaled it
-- for the balance number that is not carried forward (docs/adr/0002-reuse-from-v0.md).

local carlink = {}

local sample = {
  t = 0, d = 0, v = 0, gas = 0, brake = 0, steer = 0, lat_g = 0, lon_g = 0, yaw = 0,
  sa_fl = 0, sa_fr = 0, sa_rl = 0, sa_rr = 0,
  sr_fl = 0, sr_fr = 0, sr_rl = 0, sr_rr = 0,
  load_fl = 0, load_fr = 0, load_rl = 0, load_rr = 0,
  out = 0, rh_f = 0, rh_r = 0,
}
carlink.sample = sample

---@param car ac.StateCar
---@param trackLength number metres
---@param t number app clock, seconds
function carlink.read(car, trackLength, t)
  local s = sample
  local wheels = car.wheels
  local fl, fr, rl, rr = wheels[0], wheels[1], wheels[2], wheels[3]
  s.t = t
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
  return s
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
