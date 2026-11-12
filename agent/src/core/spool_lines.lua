-- Builds the JSON lines of the spool (contracts/spool_session.schema.json and
-- contracts/spool_pass.schema.json). The JSON is written by hand instead of through
-- JSON.stringify so a pass can be encoded one column per frame: a pass holds about 20,000
-- numbers, and encoding it in one go would be a visible stall on the game thread.
-- Pure Lua.

local channels = require("core.channels")

local lines = {}

local floor, concat, format = math.floor, table.concat, string.format

local ESCAPES = { ['"'] = '\\"', ["\\"] = "\\\\" }

local function quote(text)
  local escaped = tostring(text or ""):gsub('[%c"\\]', function(char)
    return ESCAPES[char] or format("\\u%04x", char:byte())
  end)
  return '"' .. escaped .. '"'
end

local function round(x, decimals)
  if x - x ~= 0 then return 0 end  -- NaN or infinity: not valid JSON
  local scale = 10 ^ decimals
  return floor(x * scale + 0.5) / scale
end

local function num(x, decimals) return tostring(round(x or 0, decimals)) end

-- %.0f, not tostring: LuaJIT prints large whole numbers as 1.79e+12, which is not an integer
local function int(x) return format("%.0f", round(x or 0, 0)) end

local function four(values, decimals)
  local out = {}
  for i = 1, 4 do out[i] = num(values[i], decimals) end
  return "[" .. concat(out, ",") .. "]"
end

local function cornerJson(c)
  return concat({
    '{"number":', int(c.id), ',"name":', quote(c.name), ',"direction":', quote(c.dir),
    ',"start_m":', num(c.gStart, 2), ',"end_m":', num(c.gEnd, 2),
    ',"apex_m":', num(c.apex, 2), ',"radius_m":', num(c.radius, 2),
    ',"angle_deg":', num(c.angle, 1), ',"zone_start_m":', num(c.zStart, 2),
    ',"zone_end_m":', num(c.zEnd, 2), "}",
  })
end

---The first line of a session: car, track and the corner map.
---@param meta table sessionId, startedAtMs, source, agentVersion, carId, carName, trackId, layout, trackName, mapVersion
---@param map table corner map from corners.build
function lines.session(meta, map)
  local cornerList = {}
  for i, c in ipairs(map.list) do cornerList[i] = cornerJson(c) end
  return concat({
    '{"kind":"session","v":1,"session_id":', quote(meta.sessionId),
    ',"started_at_ms":', int(meta.startedAtMs), ',"source":', quote(meta.source),
    ',"agent_version":', quote(meta.agentVersion),
    ',"car":{"ac_id":', quote(meta.carId), ',"name":', quote(meta.carName), "}",
    ',"track":{"ac_id":', quote(meta.trackId), ',"layout":', quote(meta.layout),
    ',"name":', quote(meta.trackName), ',"length_m":', num(map.L, 2),
    ',"corner_map_version":', int(meta.mapVersion),
    ',"corners":[', concat(cornerList, ","), "]}}",
  })
end

local function parameterJson(item)
  local parts = {
    '{"name":', quote(item.name), ',"label":', quote(item.label), ',"tab":', quote(item.tab),
    ',"min":', num(item.min, 4), ',"max":', num(item.max, 4), ',"step":', num(item.step, 4),
    ',"shown_scale":', num(item.mult or 1, 6),
    ',"excluded":', item.auto and "false" or "true",
  }
  for _, field in ipairs({ "kind", "role", "axle" }) do
    if item[field] then
      parts[#parts + 1] = ',"' .. field .. '":'
      parts[#parts + 1] = quote(item[field])
    end
  end
  parts[#parts + 1] = "}"
  return concat(parts)
end

---The car's setup menu, once per session.
---@param sessionId string
---@param cat table catalog from catalog.build
function lines.catalog(sessionId, cat)
  local items = {}
  for i, item in ipairs(cat.items) do items[i] = parameterJson(item) end
  return concat({
    '{"kind":"catalog","v":1,"session_id":', quote(sessionId),
    ',"parameters":[', concat(items, ","), "]}",
  })
end

---The setup on the car: its fingerprint and every value that makes it up.
---@param sessionId string
---@param hash string from versions.key
---@param values table item name -> raw value
function lines.setup(sessionId, hash, values)
  local names = {}
  for name in pairs(values) do names[#names + 1] = name end
  table.sort(names)
  local pairsJson = {}
  for i, name in ipairs(names) do pairsJson[i] = quote(name) .. ":" .. num(values[name], 4) end
  return concat({
    '{"kind":"setup","v":1,"session_id":', quote(sessionId), ',"setup_hash":', quote(hash),
    ',"values":{', concat(pairsJson, ","), "}}",
  })
end

local function conditionsJson(c)
  return concat({
    ',"conditions":{"fuel_l":', num(c.fuel, 2),
    ',"tyre_core_c":', four(c.core, 1), ',"tyre_surface_c":', four(c.surface, 1),
    ',"tyre_press_psi":', four(c.pressure, 2), ',"tyre_wear":', four(c.wear, 4),
    ',"grip":', num(c.grip, 4), ',"air_c":', num(c.air, 1), ',"road_c":', num(c.road, 1), "}",
  })
end

local function headJson(raw, meta)
  local parts = {
    '{"kind":"pass","v":1,"session_id":', quote(meta.sessionId), ',"seq":', int(meta.seq),
    ',"corner":', int(raw.corner.id), ',"t_start_ms":', int(meta.startMs),
    ',"duration_s":', num(raw.tOut - raw.tIn, 4),
  }
  if meta.lap then
    parts[#parts + 1] = ',"lap_in_session":'
    parts[#parts + 1] = int(meta.lap)
  end
  if meta.setupHash then
    parts[#parts + 1] = ',"setup_hash":'
    parts[#parts + 1] = quote(meta.setupHash)
  end
  if meta.block then
    parts[#parts + 1] = concat({
      ',"block":{"experiment_id":', int(meta.block.experimentId), ',"order":', int(meta.block.order),
      ',"lap":', int(meta.block.lap), "}",
    })
  end
  if meta.conditions then parts[#parts + 1] = conditionsJson(meta.conditions) end
  return concat(parts)
end

local scratch = {}  -- reused for every column: no garbage per pass

local function columnJson(name, values, n, decimals, offset)
  local scale = 10 ^ decimals
  for i = 1, n do
    local x = values[i] - offset
    if x - x ~= 0 then x = 0 end
    scratch[i] = floor(x * scale + 0.5) / scale
  end
  return '"' .. name .. '":[' .. concat(scratch, ",", 1, n) .. "]"
end

---Start encoding a finished pass. Call lines.step until it returns the line.
---@param raw table finished pass from recorder:feed (corner, n, cols, tIn, tOut)
---@param meta table sessionId, seq, startMs, lap?, setupHash?, block?, conditions?
function lines.beginPass(raw, meta)
  return { raw = raw, head = headJson(raw, meta), encoded = {}, done = 0 }
end

---Encode one more column. Returns the finished line after the last one, nil before that.
function lines.step(job)
  local raw = job.raw
  local channel = channels.list[job.done + 1]
  if channel then
    local name, decimals = channel[1], channel[2]
    local offset = name == "t" and raw.tIn or 0
    job.done = job.done + 1
    job.encoded[job.done] = columnJson(name, raw.cols[name], raw.n, decimals, offset)
    return nil
  end
  return concat({
    job.head, ',"samples":{"n":', int(raw.n), ',"columns":{', concat(job.encoded, ","), "}}}",
  })
end

---Encode the rest of a pass in one go (used when the next pass is already waiting).
function lines.finish(job)
  local line = lines.step(job)
  while not line do line = lines.step(job) end
  return line
end

return lines
