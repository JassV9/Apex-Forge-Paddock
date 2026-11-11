-- Corner pass recorder. Fed one telemetry sample per frame; records a pass only from the
-- moment the car crosses a zone's start until it crosses that zone's end, so restarts, resets,
-- pit stops and teleports simply drop the pass in progress. No laps involved anywhere.
-- Pure Lua.

local channels = require("core.channels")
local corners = require("core.corners")

local recorder = {}
recorder.__index = recorder

-- sample fields copied into column buffers
recorder.FIELDS = channels.names

local MAX_SAMPLES = 2400   -- 40 s at 60 Hz, far more than any corner zone
local MIN_DT = 0.9 / 60    -- record at most ~60 samples per second (0.9: frame-time jitter)

---@param map table corner map from corners.build
function recorder.new(map)
  local self = setmetatable({}, recorder)
  self.map = map
  self.L = map.L
  -- two column buffers: a finished pass stays readable while the next zone records
  self.bufs = {}
  for b = 1, 2 do
    self.bufs[b] = {}
    for _, f in ipairs(recorder.FIELDS) do self.bufs[b][f] = {} end
  end
  self.buf = 1
  self.cols = self.bufs[1]
  self.n = 0
  self.zone = nil        -- corner being recorded
  self.prevD = nil
  self.prevT = nil
  self.lastRecT = -1
  self.aborts = {}       -- reason -> count (for the debug log)
  return self
end

function recorder:abort(reason)
  if self.zone then
    self.aborts[reason] = (self.aborts[reason] or 0) + 1
  end
  self.zone = nil
  self.n = 0
end

---Forget position history (after a teleport, restart or pit visit) and drop any pass.
function recorder:reset(reason)
  self:abort(reason or "reset")
  self.prevD = nil
  self.prevT = nil
end

function recorder:push(s)
  local n = self.n + 1
  if n > MAX_SAMPLES then return end
  local cols = self.cols
  for _, f in ipairs(recorder.FIELDS) do cols[f][n] = s[f] or 0 end
  self.n = n
  self.lastRecT = s.t
end

-- does moving from a to b (forward, by delta) cross point p?
local function crosses(a, delta, p, L)
  local off = corners.fwd(a, p, L)
  return off < delta, off
end

---Feed one sample. `s.t` seconds, `s.d` metres from the start line, `s.v` m/s, plus the
---fields in recorder.FIELDS. Returns a finished raw pass `{corner, n, cols, tIn, tOut}` or nil.
---@param s table
---@param ok boolean false when the sample must not count (pits, paused, replay...)
function recorder:feed(s, ok)
  if not ok then
    self:reset("not driving")
    return nil
  end
  local L = self.L
  local prevD, prevT = self.prevD, self.prevT
  self.prevD, self.prevT = s.d, s.t
  if not prevD then return nil end
  local dt = s.t - prevT
  if dt <= 0 then return nil end

  local delta = s.d - prevD
  if delta < -L / 2 then delta = delta + L end  -- crossed the line
  if delta > L / 2 then delta = delta - L end
  -- jumps: the car moved much further than its speed allows, or went backwards
  local maxMove = 2 * math.max(s.v, 1) * dt + 5
  if delta > maxMove then
    self:reset("jump")
    self.prevD, self.prevT = s.d, s.t
    return nil
  end
  if delta < -2 then
    self:abort("backwards")
    return nil
  end
  if delta <= 0 then return nil end

  local finished = nil
  if self.zone then
    local z = self.zone
    local hit, off = crosses(prevD, delta, z.zEnd, L)
    if hit then
      -- interpolate the exit time and close the pass
      local tOut = prevT + dt * (off / delta)
      self:push(s)
      finished = { corner = z, n = self.n, cols = self.cols, tIn = self.tIn, tOut = tOut }
      self.zone = nil
      self.buf = 3 - self.buf
      self.cols = self.bufs[self.buf]
    else
      if s.t - self.tIn > 40 then
        self:abort("too long")
      elseif s.t - self.lastRecT >= MIN_DT then
        self:push(s)
      end
    end
  end
  if not self.zone then
    for _, c in ipairs(self.map.list) do
      local hit, off = crosses(prevD, delta, c.zStart, L)
      if hit then
        self.zone = c
        self.n = 0
        self.tIn = prevT + dt * (off / delta)
        self:push(s)
        break
      end
    end
  end
  return finished
end

return recorder
