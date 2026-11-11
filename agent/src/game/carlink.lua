-- Reads the player's car once per frame into one reused sample table (no garbage per frame),
-- and keeps slow running averages of tyre temperatures and hot pressures for the tyre advice.

local carlink = {}

local abs, max = math.abs, math.max

local sample = {
  t = 0, d = 0, v = 0, gas = 0, brake = 0, steer = 0, latG = 0, lonG = 0, yaw = 0,
  aF = 0, aR = 0, ndF = 0, ndR = 0, srF = 0, srR = 0, out = 0, rhF = 0, rhR = 0, aFmax = 0, aRmax = 0,
}
carlink.sample = sample

-- tyres: exponential averages while cornering hard (time constant ~45 s of cornering)
local TAU = 45
carlink.tyres = {}
for i = 0, 3 do carlink.tyres[i] = { inner = 0, mid = 0, outer = 0, core = 0, press = 0, w = 0 } end
carlink.coreAvg = 0

local function axleSlip(wa, wb)
  local la, lb = max(wa.load, 0), max(wb.load, 0)
  local sa, sb = abs(wa.slipAngle), abs(wb.slipAngle)
  if la + lb > 1 then return (sa * la + sb * lb) / (la + lb) end
  return (sa + sb) * 0.5
end

-- front/rear peak slip angles from tyres.ini: balance is compared in "share of peak" so a
-- tyre that peaks at a bigger angle doesn't read as understeer (scaled back to degrees)
carlink.scaleF, carlink.scaleR = 1, 1
function carlink.setPeaks(pf, pr)
  if pf and pr and pf > 0 and pr > 0 then
    local avg = (pf + pr) / 2
    carlink.scaleF, carlink.scaleR = avg / pf, avg / pr
  else
    carlink.scaleF, carlink.scaleR = 1, 1
  end
end

---@param car ac.StateCar
---@param L number track length
---@param t number app clock (s)
function carlink.read(car, L, t)
  local s = sample
  local w0, w1, w2, w3 = car.wheels[0], car.wheels[1], car.wheels[2], car.wheels[3]
  s.t = t
  s.d = car.splinePosition * L
  s.v = car.speedMs
  s.gas, s.brake, s.steer = car.gas, car.brake, car.steer
  s.latG, s.lonG = car.acceleration.x, car.acceleration.z
  s.yaw = car.localAngularVelocity.y
  s.aF, s.aR = axleSlip(w0, w1) * carlink.scaleF, axleSlip(w2, w3) * carlink.scaleR
  -- the worst single tyre per axle: an averaged axle can hide one saturated tyre
  s.aFmax = max(abs(w0.slipAngle), abs(w1.slipAngle)) * carlink.scaleF
  s.aRmax = max(abs(w2.slipAngle), abs(w3.slipAngle)) * carlink.scaleR
  s.ndF, s.ndR = (w0.ndSlip + w1.ndSlip) * 0.5, (w2.ndSlip + w3.ndSlip) * 0.5
  s.srF = max(abs(w0.slipRatio), abs(w1.slipRatio))
  s.srR = max(abs(w2.slipRatio), abs(w3.slipRatio))
  s.out = car.wheelsOutside
  s.rhF, s.rhR = car.rideHeight[0], car.rideHeight[1]
  return s
end

---Tyre averages, only while cornering hard (temps on a straight say little about camber).
function carlink.updateTyres(car, dt)
  if dt <= 0 or abs(car.acceleration.x) < 0.6 or car.speedKmh < 60 then return end
  local k = math.min(1, dt / TAU)
  local core = 0
  for i = 0, 3 do
    local w, a = car.wheels[i], carlink.tyres[i]
    local kk = a.w < 2 and 0.5 or k   -- settle fast on the first samples
    a.inner = a.inner + (w.tyreInsideTemperature - a.inner) * kk
    a.mid = a.mid + (w.tyreMiddleTemperature - a.mid) * kk
    a.outer = a.outer + (w.tyreOutsideTemperature - a.outer) * kk
    a.core = a.core + (w.tyreCoreTemperature - a.core) * kk
    a.press = a.press + (w.tyrePressure - a.press) * kk
    a.w = a.w + dt
    core = core + w.tyreCoreTemperature
  end
  carlink.coreAvg = core / 4
end

function carlink.resetTyres()
  for i = 0, 3 do
    local a = carlink.tyres[i]
    a.inner, a.mid, a.outer, a.core, a.press, a.w = 0, 0, 0, 0, 0, 0
  end
end

---Are the tyres far below their working window right now? (out-lap / first corners)
function carlink.coldTyres(car)
  local opt = car.wheels[0].tyreOptimumTemperature
  if not opt or opt < 40 then return false end
  local core = 0
  for i = 0, 3 do core = core + car.wheels[i].tyreCoreTemperature end
  return core / 4 < opt - 25
end

---What the car data says about its tyres: ideal hot pressure and peak slip angle per axle
---for the fitted compound (tyres.ini works even inside data.acd).
function carlink.tyreData(car)
  local ok, ini = pcall(ac.INIConfig.carData, 0, "tyres.ini")
  if not ok or not ini then return nil end
  local idx = car.compoundIndex or 0
  local suffix = idx > 0 and ("_" .. idx) or ""
  local function get(sec, key)
    local okGet, v = pcall(ini.get, ini, sec .. suffix, key, 0)
    v = okGet and tonumber(v) or 0
    return v
  end
  local out = {
    F = { ideal = get("FRONT", "PRESSURE_IDEAL"), peakSlip = get("FRONT", "FRICTION_LIMIT_ANGLE") },
    R = { ideal = get("REAR", "PRESSURE_IDEAL"), peakSlip = get("REAR", "FRICTION_LIMIT_ANGLE") },
    compound = idx,
  }
  if out.F.ideal <= 0 and out.R.ideal <= 0 then return nil end
  return out
end

---Tyre summary for the advice and the LLM pack (needs ~20 s of hard cornering).
function carlink.tyreSummary(data)
  local T = carlink.tyres
  if T[0].w < 20 then return nil end
  local function axle(a, b) return (T[a].press + T[b].press) * 0.5 end
  local res = {
    F = { hot = axle(0, 1), ideal = data and data.F.ideal or nil },
    R = { hot = axle(2, 3), ideal = data and data.R.ideal or nil },
    lines = {},
  }
  local names = { [0] = "LF", "RF", "LR", "RR" }
  for i = 0, 3 do
    local a = T[i]
    res.lines[#res.lines + 1] = string.format("%s: inner %.0f / middle %.0f / outer %.0f C, core %.0f C, hot %.1f psi",
      names[i], a.inner, a.mid, a.outer, a.core, a.press)
  end
  if data then
    res.lines[#res.lines + 1] = string.format("Ideal hot pressure from the car's tyres.ini: front %.1f, rear %.1f psi.",
      data.F.ideal, data.R.ideal)
  end
  return res
end

return carlink
