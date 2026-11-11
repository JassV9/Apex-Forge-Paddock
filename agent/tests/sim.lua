-- Synthetic car and driver for offline tests: drives a line with a grip-limited speed profile
-- and plausible pedal, steering and slip traces at 60 Hz.
-- Ported from Apex Forge v0 `tests/sim.lua`; samples use Paddock's channel names and per-wheel
-- values (core/channels.lua).

local sim = {}

local G = 9.81

local function wrapAngle(a)
  while a > math.pi do a = a - 2 * math.pi end
  while a < -math.pi do a = a + 2 * math.pi end
  return a
end

---@param pts {x: number, z: number}[] evenly spaced points of a closed line
---@param L number lap length
function sim.track(pts, L)
  local n = #pts
  local ds = L / n
  local head, k = {}, {}
  for i = 1, n do
    local a, b = pts[i], pts[i % n + 1]
    head[i] = math.atan2(b.z - a.z, b.x - a.x)
  end
  for i = 1, n do k[i] = wrapAngle(head[i] - head[(i - 2) % n + 1]) / ds end
  local smooth = {}
  for i = 1, n do
    local sum = 0
    for j = -4, 4 do sum = sum + k[(i - 1 + j) % n + 1] end
    smooth[i] = sum / 9
  end
  return { n = n, ds = ds, L = L, k = smooth }
end

---Speed profile for a car: grip mu (g), braking b (g), acceleration a (g), top speed vmax (m/s).
function sim.profile(tr, car)
  local n, ds = tr.n, tr.ds
  local v = {}
  for i = 1, n do
    local kk = math.abs(tr.k[i])
    v[i] = kk > 1e-5 and math.min(car.vmax, math.sqrt(car.mu * G / kk)) or car.vmax
  end
  for _ = 1, 2 do  -- two passes so the profile wraps cleanly over the start line
    for j = 2 * n, 1, -1 do
      local i, nx = (j - 1) % n + 1, j % n + 1
      local limit = math.sqrt(v[nx] ^ 2 + 2 * car.b * G * ds)
      if v[i] > limit then v[i] = limit end
    end
    for j = 1, 2 * n do
      local i, nx = (j - 1) % n + 1, j % n + 1
      local limit = math.sqrt(v[i] ^ 2 + 2 * car.a * G * ds)
      if v[nx] > limit then v[nx] = limit end
    end
  end
  return v
end

local function lerp(a, b, t) return a + (b - a) * t end

-- pedals for this instant; returns gas, brake
local function pedals(o, car, state, dt, wantBrake, latG, speedingUp)
  if wantBrake > 0.05 then
    local brake = math.min(1, state.brake + dt / (o.attack or 0.1))
    brake = math.min(brake, 0.15 + wantBrake)
    if math.abs(latG) > 0.6 * car.mu then brake = math.min(brake, (o.trail or 0.5) * 0.6 + 0.05) end
    return 0, brake
  end
  local brake = math.max(0, state.brake - dt / 0.25)
  local gas = math.min(speedingUp and 1 or 0.35, state.gas + dt / (o.toFull or 0.5))
  if brake > 0.02 then gas = 0 end
  return gas, brake
end

local function wheelSample(o, car, state, speed, kk, latG, d)
  local load = math.min(1, math.abs(latG) / car.mu)
  local slip = (o.slip0 or 3.0) * load
  local lockFront = o.lock and state.brake > 0.5 and o.lock(d)
  local spinRear = o.spin and state.gas > 0.6 and o.spin(d)
  local srF = lockFront and 0.3 or 0.05 * state.brake
  local srR = spinRear and 0.3 or 0.04 * state.brake
  local outside = (o.off and o.off(d)) and 4 or 0
  return {
    v = speed, gas = state.gas, brake = state.brake, lat_g = latG, lon_g = 0, yaw = speed * kk,
    sa_fl = slip, sa_fr = slip, sa_rl = slip, sa_rr = slip,
    sr_fl = srF, sr_fr = srF, sr_rl = srR, sr_rr = srR,
    load_fl = 3000, load_fr = 3000, load_rl = 3400, load_rr = 3400,
    out = outside, rh_f = 0.02, rh_r = 0.04,
  }
end

---Drive laps; calls feed(sample) at 60 Hz. Options: car {mu, b, a, vmax, wheelbase, ratio},
---laps, pace, trail, startD (m), t0 (s), stopAt (s), lock/spin/off = function(d) -> boolean.
---Returns the time and distance where it stopped.
function sim.drive(tr, v, o, feed)
  local car = o.car
  local n, ds, L = tr.n, tr.ds, tr.L
  local dt = 1 / 60
  local t, d = o.t0 or 0, o.startD or 0
  local total = (o.laps or 1) * L - (o.startD or 0)
  local travelled = 0
  local pace = o.pace or 1
  local state = { gas = 1, brake = 0 }
  while travelled < total do
    local i = math.floor(d / ds) % n + 1
    local i2 = i % n + 1
    local f = (d / ds) % 1
    local speed = lerp(v[i], v[i2], f) * pace
    local kk = lerp(tr.k[i], tr.k[i2], f)
    local ahead = v[(i + 2) % n + 1] * pace
    local decel = (speed - ahead) / (2 * ds / math.max(speed, 1))
    local wantBrake = decel > 2.5 and math.min(1, decel / (car.b * G)) or 0
    local latG = speed * speed * kk / G
    state.gas, state.brake = pedals(o, car, state, dt, wantBrake, latG, ahead > speed + 0.3)
    local s = wheelSample(o, car, state, speed, kk, latG, d)
    s.t, s.d = t, d % L
    s.steer = math.deg(math.atan(car.wheelbase * kk)) * car.ratio
    feed(s)
    if o.stopAt and t >= o.stopAt then return t, d end
    t = t + dt
    d = (d + speed * dt) % L
    travelled = travelled + speed * dt
  end
  return t, d
end

return sim
