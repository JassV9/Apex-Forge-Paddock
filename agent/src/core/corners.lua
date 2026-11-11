-- Corner map from the track's AI line: finds corners by curvature, numbers them T1.. from the
-- start line and gives each one a zone (braking in, through the corner, traction out).
-- Pure Lua: takes a list of points sampled at even distances, returns tables.

local corners = {}

local TAU = 2 * math.pi

local function wrapAngle(a)
  while a > math.pi do a = a - TAU end
  while a < -math.pi do a = a + TAU end
  return a
end

-- distance from a to b going forward on a loop of length L
local function fwd(a, b, L)
  local d = (b - a) % L
  return d
end
corners.fwd = fwd

local function smooth(src, n, half)
  local out, w = {}, 2 * half + 1
  for i = 1, n do
    local s = 0
    for k = -half, half do s = s + src[(i - 1 + k) % n + 1] end
    out[i] = s / w
  end
  return out
end

corners.defaults = {
  kOn = 1 / 280,        -- curvature that makes it a corner (radius 280 m)
  kOff = 1 / 600,       -- curvature where a corner ends (hysteresis)
  minAngleDeg = 14,     -- smaller direction changes are kinks, not corners
  mergeGapM = 30,       -- same-direction pieces closer than this are one corner
  entryMaxM = 260,      -- zone reaches back this far to catch the braking point
  exitMaxM = 120,       -- and this far out to catch traction
  smoothM = 14,         -- curvature smoothing half-window
}

---@param pts {x:number, z:number}[] points evenly spaced along the lap, first one on the start line
---@param L number lap length in metres
---@param opts table|nil
---@return {list: table[], L: number, step: number}
function corners.build(pts, L, opts)
  local o = setmetatable(opts or {}, { __index = corners.defaults })
  local n = #pts
  assert(n >= 50, "need at least 50 points")
  local ds = L / n

  -- heading of each segment, curvature at each point
  local head = {}
  for i = 1, n do
    local a, b = pts[i], pts[i % n + 1]
    head[i] = math.atan2(b.z - a.z, b.x - a.x)
  end
  local k = {}
  for i = 1, n do
    k[i] = wrapAngle(head[i] - head[(i - 2) % n + 1]) / ds
  end
  local half = math.max(1, math.floor(o.smoothM / ds + 0.5))
  k = smooth(smooth(k, n, half), n, half)

  -- start scanning from the straightest point so no corner is cut in two by the scan start
  local start, best = 1, math.huge
  for i = 1, n do
    local a = math.abs(k[i])
    if a < best then best, start = a, i end
  end

  local raw = {}
  local cur = nil
  for j = 0, n - 1 do
    local i = (start - 1 + j) % n + 1
    local ki = k[i]
    local sgn = ki > 0 and 1 or -1
    if cur then
      if math.abs(ki) >= o.kOff and sgn == cur.sign then
        cur.last = j
        cur.turn = cur.turn + ki * ds
        if math.abs(ki) > cur.kMax then cur.kMax, cur.apexJ = math.abs(ki), j end
      else
        raw[#raw + 1] = cur
        cur = nil
      end
    end
    if not cur and math.abs(ki) >= o.kOn then
      -- walk back to where it crossed kOff
      local first = j
      while first > 0 do
        local p = (start - 1 + first - 1) % n + 1
        if math.abs(k[p]) < o.kOff or (k[p] > 0) ~= (ki > 0) then break end
        first = first - 1
      end
      cur = { first = first, last = j, sign = sgn, turn = 0, kMax = math.abs(ki), apexJ = j }
      for q = first, j do cur.turn = cur.turn + k[(start - 1 + q) % n + 1] * ds end
    end
  end
  if cur then raw[#raw + 1] = cur end

  -- merge same-direction pieces split by a short gap
  local merged = {}
  for _, c in ipairs(raw) do
    local prev = merged[#merged]
    if prev and prev.sign == c.sign and (c.first - prev.last) * ds <= o.mergeGapM then
      prev.last = c.last
      prev.turn = prev.turn + c.turn
      if c.kMax > prev.kMax then prev.kMax, prev.apexJ = c.kMax, c.apexJ end
    else
      merged[#merged + 1] = c
    end
  end

  -- drop kinks; convert scan indices to distances from the start line
  local list = {}
  for _, c in ipairs(merged) do
    local angle = math.deg(math.abs(c.turn))
    if angle >= o.minAngleDeg then
      local function dist(j) return (((start - 1 + j) % n) * ds) % L end
      list[#list + 1] = {
        gStart = dist(c.first),
        gEnd = dist(c.last + 1),
        apex = dist(c.apexJ),
        dir = c.sign > 0 and "R" or "L",  -- AC's x/z axes: heading increasing = right turn (checked on Spa, Monza, COTA)
        angle = angle,
        radius = 1 / c.kMax,
      }
    end
  end
  table.sort(list, function(a, b) return a.gStart < b.gStart end)

  -- sorted by start distance, T1 is the first corner after the line; a corner straddling the
  -- line starts near the end of the lap, so it is the last one

  -- zones: split the straight between neighbours, exit part first
  local m = #list
  for i, c in ipairs(list) do
    c.id = i
    c.name = "T" .. i
    local prev = list[(i - 2) % m + 1]
    local nxt = list[i % m + 1]
    local gapIn = m > 1 and fwd(prev.gEnd, c.gStart, L) or L / 2
    local gapOut = m > 1 and fwd(c.gEnd, nxt.gStart, L) or L / 2
    local prevExit = math.min(o.exitMaxM, gapIn * 0.35)
    local entry = math.min(o.entryMaxM, gapIn - prevExit)
    local exit = math.min(o.exitMaxM, gapOut * 0.35)
    c.zStart = (c.gStart - math.max(0, entry)) % L
    c.zEnd = (c.gEnd + exit) % L
    c.zLen = fwd(c.zStart, c.zEnd, L)
    -- offsets inside the zone (metres from zone start)
    c.oStart = fwd(c.zStart, c.gStart, L)
    c.oApex = fwd(c.zStart, c.apex, L)
    c.oEnd = fwd(c.zStart, c.gEnd, L)
  end
  return { list = list, L = L, step = ds }
end

---Which zone is distance d (metres from the line) in? Returns corner or nil.
function corners.zoneAt(map, d)
  for _, c in ipairs(map.list) do
    if fwd(c.zStart, d, map.L) < c.zLen then return c end
  end
  return nil
end

---Speed class from the typical minimum speed through the corner.
function corners.speedClass(vMinKmh)
  if not vMinKmh then return "?" end
  if vMinKmh < 110 then return "slow" end
  if vMinKmh < 190 then return "medium" end
  return "fast"
end

return corners
