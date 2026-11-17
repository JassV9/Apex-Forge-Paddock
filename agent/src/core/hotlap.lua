-- The hotlap display's logic (docs/adr/0019-ai-reference-driver.md): after each corner pass,
-- its time against the driver's best on this setup and against the AI's median on this setup
-- from core, and what the pass's own samples show (a lockup, wheelspin, off track). The
-- thresholds are the engine's (engine/paddock_engine/features/inputs.py), so the display
-- and the dashboard name the same events. Pure Lua.

local hotlap = {}
hotlap.__index = hotlap

hotlap.SHOW_S = 6   -- a pass stays on screen this long
hotlap.FADE_S = 1   -- and fades out over the last second of it

local LOCK_SLIP_RATIO = 0.18   -- slip ratio counted as a lockup or wheelspin
local LOCK_RUN_SAMPLES = 3     -- in a row, about 50 ms
local OFF_TRACK_WHEELS = 3

local function larger(a, b) return math.max(math.abs(a), math.abs(b)) end

-- run counters: an event counts once its run reaches LOCK_RUN_SAMPLES samples
local function step(run, on) if on then return run + 1 end return 0 end

---What the samples of one finished pass show, in a fixed order; empty for a clean pass.
---@param raw table finished pass from recorder:feed (n, cols)
function hotlap.verdict(raw)
  local c = raw.cols
  local front, rear, spin, off = 0, 0, 0, false
  local seen = {}
  for i = 1, raw.n do
    local braking = c.brake[i] > 0.2 and c.gas[i] < 0.1
    local powered = c.gas[i] > 0.3 and c.brake[i] < 0.04
    local frontRatio, rearRatio = larger(c.sr_fl[i], c.sr_fr[i]), larger(c.sr_rl[i], c.sr_rr[i])
    front = step(front, braking and frontRatio > LOCK_SLIP_RATIO)
    rear = step(rear, braking and rearRatio > LOCK_SLIP_RATIO)
    spin = step(spin, powered and math.max(frontRatio, rearRatio) > LOCK_SLIP_RATIO)
    if front == LOCK_RUN_SAMPLES then seen.front = true end
    if rear == LOCK_RUN_SAMPLES then seen.rear = true end
    if spin == LOCK_RUN_SAMPLES then seen.spin = true end
    if c.out[i] >= OFF_TRACK_WHEELS then off = true end
  end
  local tags = {}
  if off then tags[#tags + 1] = "off track" end
  if seen.front then tags[#tags + 1] = "front lock" end
  if seen.rear then tags[#tags + 1] = "rear lock" end
  if seen.spin then tags[#tags + 1] = "wheelspin" end
  return tags
end

function hotlap.new()
  return setmetatable({ best = {}, names = {}, ai = {}, aiHash = nil, last = nil }, hotlap)
end

local function key(setupHash, corner) return tostring(setupHash or "?") .. "#" .. corner end

---The AI's times for one setup, as core's GET /reference answered (nil: none known).
function hotlap:setReference(setupHash, reply)
  self.ai, self.aiHash = {}, setupHash
  for _, c in ipairs(reply and reply.corners or {}) do
    self.ai[c.corner] = { median = c.median_s, margin = c.margin_s, n = c.n }
  end
end

---The AI's time of a corner on this setup, or nil.
function hotlap:aiOf(corner, setupHash)
  if setupHash == nil or setupHash ~= self.aiHash then return nil end
  return self.ai[corner]
end

---Record a finished pass; returns what the display shows of it.
function hotlap:pass(corner, name, time, setupHash, tags, now)
  local k = key(setupHash, corner)
  local best, ai = self.best[k], self:aiOf(corner, setupHash)
  self.last = {
    corner = corner, name = name, time = time, at = now, verdict = table.concat(tags, ", "),
    deltaBest = best and time - best or nil,
    deltaAI = ai and time - ai.median or nil, margin = ai and ai.margin or nil,
  }
  if not best or time < best then self.best[k] = time end
  self.names[corner] = name
  return self.last
end

---How visible the last pass is: 1, fading to 0 over the last FADE_S of SHOW_S.
function hotlap:alpha(now)
  if not self.last then return 0 end
  local age = now - self.last.at
  if age < 0 or age >= hotlap.SHOW_S then return 0 end
  return math.min(1, (hotlap.SHOW_S - age) / hotlap.FADE_S)
end

---"driver" when the gap to the AI is beyond its margin, "ahead" when faster by more, else "even".
function hotlap.against(deltaAI, margin)
  if deltaAI == nil then return nil end
  if deltaAI > margin then return "driver" end
  if deltaAI < -margin then return "ahead" end
  return "even"
end

---One row per corner with a best on this setup or an AI time, by corner number.
function hotlap:rows(setupHash)
  local numbers, seen = {}, {}
  local function add(n) if not seen[n] then seen[n], numbers[#numbers + 1] = true, n end end
  for n in pairs(self.names) do if self.best[key(setupHash, n)] then add(n) end end
  if setupHash ~= nil and setupHash == self.aiHash then for n in pairs(self.ai) do add(n) end end
  table.sort(numbers)
  local rows = {}
  for i, n in ipairs(numbers) do
    local best, ai = self.best[key(setupHash, n)], self:aiOf(n, setupHash)
    rows[i] = { corner = n, name = self.names[n] or ("T" .. n), best = best,
      ai = ai and ai.median or nil, margin = ai and ai.margin or nil,
      delta = best and ai and best - ai.median or nil }
  end
  return rows
end

function hotlap.seconds(x) if x == nil then return "--" end return string.format("%.3f", x) end

function hotlap.signed(x) if x == nil then return "--" end return string.format("%+.3f", x) end

local function escape(ch) return string.format("%%%02X", ch:byte()) end

local function encode(text) return (tostring(text or ""):gsub("[^%w%-%._~]", escape)) end

local TEST_COPY_SUFFIX = "_paddock"  -- the layout copy made by tools/prepare_test_layout.py

---Why there are no AI times to show, in words. The AI drives only the "(Paddock test)" copy of
---a layout, so on any other layout there never are any.
---@param layout string ac.getTrackLayout()
function hotlap.noReferenceText(layout)
  if layout:sub(-#TEST_COPY_SUFFIX) == TEST_COPY_SUFFIX then
    return "The AI has not driven this setup here yet."
  end
  return "No AI times: this is the original layout, and the AI drives only its \"(Paddock test)\""
    .. " copy. Drive that copy to be compared."
end

---The path of core's GET /reference for this car, track, layout and setup.
function hotlap.referencePath(car, track, layout, setupHash)
  return "/reference?car=" .. encode(car) .. "&track=" .. encode(track)
    .. "&layout=" .. encode(layout) .. "&setup_hash=" .. encode(setupHash)
end

return hotlap
