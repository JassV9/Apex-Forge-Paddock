-- Reads an LLM's answer (pasted from the clipboard) and turns it into a checked change list:
-- every item must exist on this car and be in the pack, every value must be legal. Values
-- written as the menu shows them (-2.6 deg) are converted to setup units (-26) when that's
-- the only sensible reading. Pure Lua.

local catalog = require("core.catalog")

local reply = {}

local function trim(s) return (s:gsub("^%s+", ""):gsub("%s+$", "")) end

-- pull NAME=value pairs out of the text; prefers the ```apexforge block
local function pairsFrom(text)
  local body = text:match("```%s*[Aa][Pp][Ee][Xx][Ff][Oo][Rr][Gg][Ee][^\n]*\n(.-)```")
  local fromBlock = body ~= nil
  body = body or text
  local out, title = {}, nil
  local section = nil
  for raw in (body .. "\n"):gmatch("([^\n]*)\n") do
    local line = trim((raw:gsub("%s*[;#].*$", "")))
    local t = line:match("^[Tt][Ii][Tt][Ll][Ee]%s*[:=]%s*(.+)$") or line:match("^[Nn][Aa][Mm][Ee]%s*:%s*(.+)$")
    if t then
      title = trim(t)
    else
      local sec = line:match("^%[([%w_]+)%]$")
      if sec then
        section = sec
      else
        local k, v = line:match("^[%-%*%s`]*([%w_]+)[`%s]*[=:]%s*(.-)%s*$")
        if k and v and v ~= "" then
          if section and k:upper() == "VALUE" then
            out[#out + 1] = { name = section, text = v }
            section = nil
          elseif k:upper() ~= "VALUE" then
            out[#out + 1] = { name = k, text = v }
          end
        end
      end
    end
  end
  return out, title, fromBlock
end

-- setup files call the compound section TYRES, the setup menu calls it COMPOUND
local ALIAS = { TYRES = "COMPOUND" }

local function findItem(cat, name)
  local it = cat.byName[name] or cat.byName[name:upper()] or cat.byName[ALIAS[name:upper()] or ""]
  if it then return it, nil end
  local want = name:upper():gsub("_", " ")
  local hit, count = nil, 0
  for _, i in ipairs(cat.items) do
    if i.included and (i.label or ""):upper() == want then hit, count = i, count + 1 end
  end
  if count == 1 then return hit, "matched by label" end
  return nil, count > 1 and "label is ambiguous" or nil
end

---@return number|nil value, string|nil kind, string|nil err
local function parseValue(it, text)
  -- unicode minus and dashes (LLMs like them) -> "-"
  local t = trim((text:gsub("[`\"']", ""):gsub("\226\136\146", "-"):gsub("\226\128[\147\148]", "-")))
  -- "56 -> 55" / "56 to 55": the new value is the last one
  t = t:match(".*%->%s*(.-)$") or t:match(".*\226\134\146%s*(.-)$") or t:match("^[%-%d%.,]+%s+to%s+(.-)$") or t
  t = trim(t)
  if it.options then
    local want = t:lower()
    for i, o in ipairs(it.options) do
      if want == o:lower() then return catalog.optionValue(it, i), "option" end
    end
    if t:find("[%a/]") then
      for i, o in ipairs(it.options) do
        local ol = o:lower()
        if want:find(ol, 1, true) or (#want >= 3 and ol:find(want, 1, true)) then
          return catalog.optionValue(it, i), "option"
        end
      end
    end
  end
  if t:match("^%+") then
    return nil, nil, "'" .. text .. "' looks like a relative change; the answer must give the new value"
  end
  -- thousands separators: 97,500 -> 97500
  local prev
  repeat prev = t; t = t:gsub("(%d),(%d%d%d)", "%1%2") until t == prev
  local num = tonumber(t:match("^([%-]?%d+%.?%d*)") or t:match("^([%-]?%.%d+)") or "")
  return num, nil, nil
end

---@param text string the pasted answer
---@param cat table catalog
---@param current table name -> current value
---@return {title: string|nil, changes: table[], problems: string[], fromBlock: boolean}
function reply.parse(text, cat, current)
  local res = { title = nil, changes = {}, problems = {}, fromBlock = false }
  if type(text) ~= "string" or text:match("^%s*$") then
    res.problems[#res.problems + 1] = "Clipboard is empty."
    return res
  end
  local list, title, fromBlock = pairsFrom(text:gsub("\r", ""))
  res.title, res.fromBlock = title, fromBlock
  local seen = {}
  for _, p in ipairs(list) do
    local it, how = findItem(cat, p.name)
    if not it then
      if fromBlock then
        res.problems[#res.problems + 1] = p.name .. ": not a setup item on this car" .. (how and (" (" .. how .. ")") or "")
      end
    elseif not it.included then
      res.problems[#res.problems + 1] = it.label .. " (" .. it.name .. "): not in the pack, left alone"
    elseif seen[it.name] then
      res.problems[#res.problems + 1] = it.label .. ": listed twice, first value kept"
    else
      seen[it.name] = true
      local v, kind, err = parseValue(it, p.text)
      local cur = current[it.name] or it.value
      if v == nil then
        res.problems[#res.problems + 1] = it.label .. ": " .. (err or ("can't read value '" .. p.text .. "'"))
      elseif not kind and (v < it.min - it.step or v > it.max + it.step)
          and not (it.mult and it.mult ~= 1 and catalog.onGrid(it, math.floor(v / it.mult + 0.5), cur)) then
        -- far outside the range: probably the wrong unit, never clamp that to an extreme
        res.problems[#res.problems + 1] = string.format("%s: %s is outside %s..%s (%s to %s), left alone",
          it.label, p.text, tostring(it.min), tostring(it.max), catalog.display(it, it.min), catalog.display(it, it.max))
      else
        local note = how
        local final
        local rawOk = catalog.onGrid(it, v, cur)
        local shown = (it.mult and it.mult ~= 1) and (v / it.mult) or nil
        local shownSnap = shown and math.floor(shown + 0.5)
        local shownOk = shown and catalog.onGrid(it, shownSnap, cur) and math.abs(shown - shownSnap) < 0.02 * it.step
        if kind == "option" then
          final = v
        elseif rawOk and shownOk then
          -- both readings legal: take the one nearer the current value
          local s = shownSnap
          if math.abs(s - cur) < math.abs(v - cur) then
            final, note = s, "read as shown value " .. p.text
          else
            final = v
          end
        elseif rawOk then
          final = v
        elseif shownOk then
          final, note = shownSnap, "converted from shown value " .. p.text
        else
          final = catalog.snap(it, v)
          if v < it.min or v > it.max then
            note = "outside " .. it.min .. ".." .. it.max .. ", clamped"
          else
            note = "not on the " .. it.step .. " step grid, rounded"
          end
        end
        if final == cur then
          local why = ", no change"
          if v > it.max and cur == it.max then why = " (the maximum), can't go to " .. p.text end
          if v < it.min and cur == it.min then why = " (the minimum), can't go to " .. p.text end
          if why == ", no change" and v ~= cur and not (shownOk and shownSnap == cur) then
            why = ", and " .. p.text .. " rounds back to it on the " .. it.step .. " step grid"
          end
          res.problems[#res.problems + 1] = it.label .. ": already " .. catalog.display(it, cur) .. why
        else
          res.changes[#res.changes + 1] = {
            name = it.name, label = it.label, from = cur, to = final, note = note, item = it,
          }
        end
      end
    end
  end
  if #res.changes == 0 and #res.problems == 0 then
    res.problems[#res.problems + 1] = "No setup changes found. Copy the whole answer, including the apexforge block."
  end
  return res
end

return reply
