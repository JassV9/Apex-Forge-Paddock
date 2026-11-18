-- The run context of the spool lines (docs/adr/0028-run-context-and-ai-provenance.md): on the
-- session line the game, the agent build and the AI's settings; on every pass the tyre hint the
-- AI was given, damage, resets and how much of the pass ABS, TC and DRS acted. Written by hand
-- like the rest of a spool line (core/spool_lines.lua). Pure Lua.

local context = {}

local floor, concat, format = math.floor, table.concat, string.format

local function number(x, decimals)
  if x - x ~= 0 then return "0" end  -- NaN or infinity: not valid JSON
  if decimals == 0 then return format("%.0f", x) end
  local scale = 10 ^ decimals
  return tostring(floor(x * scale + 0.5) / scale)
end

local function quote(text)
  return '"' .. tostring(text):gsub('[%c"\\]', function(char) return format("\\u%04x", char:byte()) end) .. '"'
end

-- {"a":1,"b":"x"} from { {name, value, decimals or "string"} }, leaving out nil values
local function object(fields)
  local parts = {}
  for _, field in ipairs(fields) do
    local name, value, kind = field[1], field[2], field[3]
    if value ~= nil then
      local text = kind == "string" and quote(value) or number(value, kind)
      parts[#parts + 1] = '"' .. name .. '":' .. text
    end
  end
  return "{" .. concat(parts, ",") .. "}"
end

local function aiJson(ai)
  return object({
    { "tyre_hint_base", ai.tyreHintBase, 4 }, { "learned_share", ai.learnedShare, 4 },
    { "pinned_share", ai.pinnedShare, 4 }, { "applied_hint", ai.appliedHint, 4 },
    { "extra_grip", ai.extraGrip, 4 }, { "aero_hint", ai.aeroHint, 4 },
    { "brake_hint", ai.brakeHint, 4 }, { "level", ai.level, 4 }, { "aggression", ai.aggression, 4 },
  })
end

---The session line's "context" member, with its leading comma.
---@param c table carsCount, cspVersion?, cspBuild?, ai (tyreHintBase, learnedShare,
---pinnedShare?, appliedHint?, extraGrip?, aeroHint?, brakeHint?, level?, aggression?)
function context.session(c)
  local head = object({
    { "cars_count", c.carsCount, 0 },
    { "csp_version", c.cspVersion, "string" }, { "csp_build", c.cspBuild, 0 },
  })
  return ',"context":' .. head:sub(1, -2) .. ',"ai":' .. aiJson(c.ai) .. "}"
end

-- share of the pass's samples where a 0/1 channel was 1
local function share(values, n)
  local on = 0
  for i = 1, n do on = on + values[i] end
  return on / n
end

---The pass line's "context" member, with its leading comma.
---@param raw table finished pass (cols.abs, cols.tc, cols.drs, n)
---@param c table clock ("physics" or "app"), aiHint? (the tyre hint the AI was given), damage
---(largest zone), resets
function context.pass(raw, c)
  local cols, n = raw.cols, raw.n
  return ',"context":' .. object({
    { "clock", c.clock, "string" }, { "ai_hint", c.aiHint, 4 },
    { "damage_max", c.damage, 2 }, { "resets", c.resets, 0 },
    { "abs_share", share(cols.abs, n), 3 }, { "tc_share", share(cols.tc, n), 3 },
    { "drs_share", share(cols.drs, n), 3 },
  })
end

return context
