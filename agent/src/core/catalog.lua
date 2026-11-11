-- The car's setup items as the in-game setup menu defines them (real labels, ranges, steps,
-- tabs), filtered to what changes handling, with a guess at what each item is (front ARB,
-- rear wing...). Roles come from the in-game LABEL first: on some cars WING_1 is the front flap.
-- Pure Lua.

local catalog = {}

local upper = string.upper

-- tabs / items that are not handling: power unit, ERS, displays, cosmetics, AI, pit strategy...
-- Matched as whole words ("ERS" must not catch "DAMPERS").
catalog.SKIP_TAB = {
  "POWER UNIT", "ERS", "DEPLOY", "DEPLOYMENT", "STRAT", "STRATS", "DRIVER DEFINITIONS", "SETTINGS",
  "DISPLAY", "AUDIO", "COSMETICS", "LED", "LEDS", "PIT", "PITSTOP", "LAUNCH", "ELECTRONICS",
  "PRESCRIPTIONS", "AI",
}
catalog.SKIP_ITEM = {
  "^MGUK", "^MGUH", "^ERS", "^KERS", "^ENGINE_LIMITER", "^STEER_ASSIST", "^HALO",
}
catalog.SKIP_LABEL = {
  "DISPLAY", "VERSION", "COSMETIC", "LED", "AUDIO", "VOLUME", "DUMMY", "HALO", "SKIN",
  "LIMITER", "LAUNCH", "DEBOUNCE", "PROTECTION", "LIMIT", "MIN STARTING",
  -- ERS / deployment (FA25 labels its 580 map items "ERS_MAP"; tabs are the main filter in game)
  "ERS", "ERS MAP", "ERS MAPPER", "DEPLOY", "SPLIT START", "SPLIT END", "SUPER CLIP", "SOC",
  "BOOST", "RECHARGE", "HARVEST", "STRAT", "MGUK", "MGUH", "PU MODE", "POWER UNIT",
  -- dash, sound, cosmetics, procedures
  "ACTIVE", "TONE", "BRIGHTNESS", "BACKLIGHT", "PRIVACY", "WINDSHIELD", "HELICOPTER", "IGNITION",
  "STARTER", "STEERING WHEEL", "POPUP", "LAP", "DRS", "MIX", "GEARBOX SHIFT", "TARGET",
}

-- " WORD WORD " form: letters and digits only, single spaces, padded
local function words(text)
  return " " .. upper(text or ""):gsub("[^%w]+", " "):gsub("^ +", ""):gsub(" +$", "") .. " "
end

local function anyWord(text, list)
  local w = words(text)
  for _, p in ipairs(list) do
    if w:find(" " .. p .. " ", 1, true) then return true end
  end
  return false
end

local function anyPattern(text, list)
  for _, p in ipairs(list) do
    if text:find(p) then return true end
  end
  return false
end

---Is this item excluded from the LLM pack / advice by default?
function catalog.skipByDefault(it)
  if (it.tab or "") ~= "" and anyWord(it.tab, catalog.SKIP_TAB) then return true end
  if anyPattern(upper(it.name), catalog.SKIP_ITEM) then return true end
  if anyWord(it.label, catalog.SKIP_LABEL) then return true end
  return false
end

-- which wheel / axle an item belongs to
local function position(name, label)
  local n = upper(name)
  local w = n:match("_([LR][FR])_") or n:match("_([LR][FR])$")
  if w then return w:sub(2, 2), w:sub(1, 1) end
  local h = n:match("_H([FR])_") or n:match("_H([FR])$")
  if h then return h, nil end
  local l = " " .. upper(label or "") .. " "
  local lw = l:match("[%s%(]([LR][FR])[%s%)]")
  if lw then return lw:sub(2, 2), lw:sub(1, 1) end
  if l:find("FRONT", 1, true) or l:find(" FW", 1, true) or l:find(" HF ", 1, true) then return "F", nil end
  if l:find("REAR", 1, true) or l:find(" RW", 1, true) or l:find(" HR ", 1, true) then return "R", nil end
  if n:find("_F$") or n:find("_FRONT") or n:find("FRONT_") then return "F", nil end
  if n:find("_R$") or n:find("_REAR") then return "R", nil end
  return nil, nil
end

---Guess the item's role. `dir` = +1 when a higher value means more of the role's effect
---(stiffer, more downforce, more front brake, more locking, more pressure); nil when unknown.
function catalog.role(it)
  local n, l = upper(it.name), upper(it.label or "")
  local axle, side = position(it.name, it.label)
  local r, dir, kind = nil, nil, nil
  if n:find("^ARB") or l:find("ARB", 1, true) or l:find("ANTI.?ROLL") or l:find("SWAY", 1, true) then
    r, dir = "arb", 1
  elseif n:find("^WING_") or l:find("WING", 1, true) or l:find("FLAP", 1, true) or l:find("GURNEY", 1, true) then
    r = "wing"
    if l:find("GURNEY", 1, true) and not axle then axle = "R" end
    if not (l:find("WING", 1, true) or l:find("FLAP", 1, true) or l:find("GURNEY", 1, true)) then
      axle = nil  -- plain WING_n with no label: never guess front or rear from the number
    end
    if l:find("ACTUAT", 1, true) or l:find("MODE", 1, true) then r = "wingmode" end
    dir = (r == "wing" and axle) and 1 or nil
  elseif n == "FRONT_BIAS" or l == "BRAKE BIAS" or l == "BRAKE BALANCE" then
    r, dir = "bias", 1
  elseif n == "BRAKE_POWER_MULT" or l == "BRAKE POWER" or l == "BRAKE PRESSURE" then
    r, dir = "brakepower", 1
  elseif n == "DIFF_POWER" or l == "DIFF POWER" then
    r, dir = "diffpower", 1
  elseif n == "DIFF_COAST" or l == "DIFF COAST" then
    r, dir = "diffcoast", 1
  elseif n == "DIFF_PRELOAD" or l:find("PRELOAD", 1, true) then
    r, dir = "preload", 1
  elseif n:find("DIFF") or l:find("DIFF", 1, true) or l:find("LOCK", 1, true) then
    r = "diff"
  elseif n:find("^CAMBER") or l:find("CAMBER", 1, true) and not l:find("LIMIT", 1, true) then
    r, dir = "camber", 1   -- value is signed; more negative = lower value
  elseif n:find("^TOE") or l:find("TOE", 1, true) then
    r = "toe"
  elseif n:find("^PRESSURE") or (l:find("PRESSURE", 1, true) and side) then
    r, dir = "pressure", 1
  elseif l:find("HEAVE", 1, true) or n:find("^SPRING_RATE_H[FR]") then
    r, dir = "heave", 1
  elseif n:find("^SPRING_RATE") or n:find("^TORSION") or l:find("WHEEL RATE", 1, true)
      or l:find("TORSION", 1, true) or l:find("SPRING", 1, true) then
    r, dir = "spring", 1
  elseif n:find("DAMP") or l:find("BUMP", 1, true) and not l:find("RUBBER", 1, true) and not l:find("STOP", 1, true)
      or l:find("REBOUND", 1, true) then
    r = "damper"
    local fast = n:find("FAST") or l:find("FST", 1, true) or l:find("FAST", 1, true)
    local reb = n:find("REBOUND") or l:find("REBOUND", 1, true)
    kind = (fast and "fast" or "slow") .. (reb and "rebound" or "bump")
  elseif n:find("BUMPSTOP") or n:find("BUMP_STOP") or n:find("PACKER") or l:find("BUMP RUBBER", 1, true) then
    r = "bumpstop"
  elseif n:find("^ROD_LENGTH") or l:find("HEIGHT", 1, true) or l:find("RIDE", 1, true)
      or l:find("PUSH/PULL", 1, true) then
    r = "height"
  elseif n == "COMPOUND" or n == "TYRES" then
    r = "compound"
  elseif n == "FUEL" then
    r = "fuel"
  elseif n:find("BRAKE_DUCT") or l:find("DUCT", 1, true) then
    r = "duct"
  end
  -- a mod with a negative display multiplier stores the value inverted
  if dir and (it.mult or 1) < 0 then dir = -dir end
  return r, axle, side, dir, kind
end

---Builds the catalog.
---@param spinners table[] from ac.getSetupSpinners()
---@param tabOf fun(name: string): string|nil tab of an item (from the car's setup.ini)
---@param overrides table|nil name -> true/false (user's include / exclude choices)
function catalog.build(spinners, tabOf, overrides)
  local cat = { items = {}, byName = {}, tabs = {}, tabIndex = {} }
  overrides = overrides or {}
  for order, sp in ipairs(spinners) do
    if sp.name and sp.min ~= nil and sp.max ~= nil then
      local it = {
        order = order, name = sp.name, label = sp.label or sp.name,
        tab = (tabOf and tabOf(sp.name)) or sp.tab or "",
        min = sp.min, max = sp.max, step = (sp.step and sp.step > 0) and sp.step or 1,
        value = sp.value, default = sp.defaultValue, mult = sp.displayMultiplier or 1,
        units = sp.units, options = sp.items, optionValues = sp.itemValues,
        clicks = sp.showClicksMode, readOnly = sp.readOnly == true, visible = sp.visible ~= false,
      }
      it.role, it.axle, it.side, it.dir, it.kind = catalog.role(it)
      it.editable = (not it.readOnly) and it.visible and it.max > it.min
      it.auto = it.editable and not catalog.skipByDefault(it)
      cat.items[#cat.items + 1] = it
      cat.byName[it.name] = it
      local tabKey = it.tab ~= "" and it.tab or "(general)"
      local t = cat.tabIndex[tabKey]
      if not t then
        t = { name = tabKey, items = {} }
        cat.tabIndex[tabKey] = t
        cat.tabs[#cat.tabs + 1] = t
      end
      t.items[#t.items + 1] = it
    end
  end
  -- if the car's setup.ini gives tabs but not for a script item, we can't tell what that item
  -- is for (could be ERS): leave it out by default
  local anyTab = false
  for _, it in ipairs(cat.items) do if it.tab ~= "" then anyTab = true break end end
  for _, it in ipairs(cat.items) do
    if anyTab and it.tab == "" and it.name:find("^CUSTOM_SCRIPT_ITEM") then it.auto = false end
    if overrides[it.name] ~= nil then it.included = overrides[it.name] and it.editable else it.included = it.auto end
  end
  return cat
end

function catalog.included(cat)
  local out = {}
  for _, it in ipairs(cat.items) do if it.included then out[#out + 1] = it end end
  return out
end

---Nearest legal value on the item's grid, clamped to its range. (For list items the value is
---the option's position: min, min+step, ...; `itemValues` are only what each position means.)
function catalog.snap(it, v)
  local k = math.floor((v - it.min) / it.step + 0.5)
  local out = it.min + k * it.step
  if out < it.min then out = it.min end
  if out > it.max then out = it.min + math.floor((it.max - it.min) / it.step) * it.step end
  return out
end

---Legal value? On the MIN + n*STEP grid, or on the grid through `anchor` (the in-game
---spinner steps from the current value, which isn't always on the MIN grid).
function catalog.onGrid(it, v, anchor)
  if v < it.min or v > it.max then return false end
  if catalog.snap(it, v) == v then return true end
  if anchor and not it.options then
    local k = (v - anchor) / it.step
    return math.abs(k - math.floor(k + 0.5)) < 1e-6
  end
  return false
end

local function fmtNum(x, mult)
  local decimals = 0
  if mult and mult ~= 1 then
    local m = math.abs(mult)
    if m < 1 then decimals = math.min(4, math.ceil(-math.log10(m) - 1e-9)) end
  end
  local s = string.format("%." .. decimals .. "f", x)
  if s == "-0" then s = "0" end
  return s
end

---Value as the setup menu shows it ("-2.8 deg", "Piorani SS (C4)", "click 9 of 18").
function catalog.display(it, v)
  v = v or it.value
  if v == nil then return "?" end
  if it.options then
    local o = it.options[catalog.optionIndex(it, v)]
    if o then return o end
  end
  if it.clicks == 2 then
    return string.format("click %d of %d", math.floor((v - it.min) / it.step + 0.5),
      math.floor((it.max - it.min) / it.step + 0.5))
  end
  local s = fmtNum(v * (it.mult or 1), it.mult)
  if it.units and it.units ~= "" then s = s .. " " .. it.units end
  return s
end

---1-based option position of a list item's value.
function catalog.optionIndex(it, v)
  return math.floor((v - it.min) / it.step + 0.5) + 1
end

---Raw value of the i-th option.
function catalog.optionValue(it, i)
  return it.min + (i - 1) * it.step
end

---Clicks between two values.
function catalog.clicks(it, a, b)
  return math.floor((b - a) / it.step + 0.5)
end

---Items whose values make up the setup's fingerprint: the default handling set, whatever the
---user ticks in the Items tab (so unticking an item doesn't orphan the data).
function catalog.keyItems(cat)
  local out = {}
  for _, it in ipairs(cat.items) do if it.auto then out[#out + 1] = it end end
  return out
end

---Find items by role (and axle), in menu order.
function catalog.find(cat, role, axle)
  local out = {}
  for _, it in ipairs(cat.items) do
    if it.included and it.role == role and (axle == nil or it.axle == axle) then out[#out + 1] = it end
  end
  return out
end

return catalog
