-- The car's setup through CSP: reads the setup menu's own items (labels, ranges, tabs from the
-- car's setup.ini) and puts a block's setup on the car by setting the spinners and reading
-- every value back. It never saves or overwrites a setup file: the driver's files are theirs.
-- Adapted from Apex Forge v0 `game/setup.lua` (see docs/adr/0002-reuse-from-v0.md).

local catalog = require("core.catalog")
local versions = require("core.versions")

local setup = {}

setup.cat = nil

local function has(name)
  local ok, f = pcall(function() return ac[name] end)
  return ok and f ~= nil
end

---True when the setup screen is open in the pits, the only place a setup can be changed.
function setup.editable()
  if not has("isSetupAvailableToEdit") then return false end
  local ok, v = pcall(ac.isSetupAvailableToEdit)
  return ok and v == true
end

---Build (or rebuild) the catalog from the live setup menu.
function setup.scan(overrides)
  local ok, spinners = pcall(ac.getSetupSpinners)
  if not ok or type(spinners) ~= "table" or #spinners == 0 then return nil end
  local okIni, ini = pcall(ac.INIConfig.carData, 0, "setup.ini")
  local function tabOf(name)
    if not okIni or not ini then return "" end
    local okGet, v = pcall(ini.get, ini, name, "TAB", "")
    return okGet and tostring(v or "") or ""
  end
  setup.cat = catalog.build(spinners, tabOf, overrides)
  return setup.cat
end

---Current values of the included items and the fingerprint items (one cheap call per item).
function setup.current()
  local cur = {}
  if not setup.cat then return cur end
  for _, it in ipairs(setup.cat.items) do
    if it.included or it.auto then
      local ok, v = pcall(ac.getSetupSpinnerValue, it.name, it.value or 0)
      cur[it.name] = ok and v or it.value
      it.value = cur[it.name]
    end
  end
  return cur
end

function setup.key(cur)
  if not setup.cat then return "unknown" end
  return versions.key(catalog.keyItems(setup.cat), cur or setup.current())
end

-- set every value, then read every value back; returns the ones that did not take
local function setAndVerify(changes)
  local failed = {}
  for _, ch in ipairs(changes) do
    local ok, res = pcall(ac.setSetupSpinnerValue, ch.name, ch.to)
    if not ok or res == false then failed[#failed + 1] = { name = ch.name, why = "rejected" } end
  end
  for _, ch in ipairs(changes) do
    local ok, got = pcall(ac.getSetupSpinnerValue, ch.name, -999999)
    if not ok or got ~= ch.to then
      failed[#failed + 1] = { name = ch.name, why = "reads " .. tostring(got) .. ", wanted " .. ch.to }
    end
  end
  return failed
end

-- put every value back; returns how many could not be put back
local function undo(changes)
  local stuck = 0
  for _, ch in ipairs(changes) do
    pcall(ac.setSetupSpinnerValue, ch.name, ch.from)
    local ok, got = pcall(ac.getSetupSpinnerValue, ch.name, -999999)
    if not ok or got ~= ch.from then stuck = stuck + 1 end
  end
  return stuck
end

---Put changes on the car and verify each one. If any value does not take, every change is
---undone, so the car is never left on a setup that is neither the old nor the new one.
---@param changes {name: string, from: number, to: number}[]
---@return {ok: boolean, failed: table[], message: string}
function setup.apply(changes)
  if not setup.editable() then
    return { ok = false, failed = {}, message = "Open the setup screen in the pits to load a setup." }
  end
  local failed = setAndVerify(changes)
  if #failed == 0 then
    return { ok = true, failed = {}, message = string.format("Set and checked %d value(s).", #changes) }
  end
  local stuck = undo(changes)
  local message = #failed .. " value(s) did not take, so every change was undone"
  if stuck > 0 then
    message = message .. string.format(" (%d could not be put back: check the setup)", stuck)
  end
  return { ok = false, failed = failed, message = message .. "." }
end

return setup
