-- The car's setup through CSP: reads the setup menu's own items (labels, ranges, tabs from the
-- car's setup.ini), tracks which setup file is loaded, and writes a new version by setting the
-- spinners, reading every value back, then saving it as a new file. Never overwrites a file.

local catalog = require("core.catalog")
local versions = require("core.versions")

local setup = {}

setup.cat = nil
setup.file = nil          -- full path of the last setup loaded/saved (nil = unknown)
setup.events = {}         -- recent load/save events for the log
setup.listeners = {}

local function has(name)
  local ok, f = pcall(function() return ac[name] end)
  return ok and f ~= nil
end
setup.has = has

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

function setup.fileStem()
  return versions.stem(setup.file)
end

-- CSP saves a temp copy for its own use; only files in a setups folder are the user's setups
local function isTempPath(path)
  local p = tostring(path or ""):lower()
  return p:find("setup_tmp", 1, true) ~= nil or p:find("[\\/]setups[\\/]") == nil
end

---Which file is loaded right now, before any load/save event this session (unverified in game:
---relies on the current-setup INI carrying its filename).
function setup.guessFile()
  if setup.file or not (ac.INIConfig and ac.INIConfig.currentSetup) then return end
  local ok, cfg = pcall(ac.INIConfig.currentSetup)
  if ok and cfg and cfg.filename and cfg.filename ~= "" and not isTempPath(cfg.filename) then
    setup.file = cfg.filename
  end
end

function setup.listen()
  if not has("onSetupFile") then return end
  pcall(ac.onSetupFile, function(op, filename)
    if isTempPath(filename) then return end
    setup.file = filename
    setup.events[#setup.events + 1] = { op = op, file = filename, at = os.time() }
    while #setup.events > 20 do table.remove(setup.events, 1) end
    for _, f in ipairs(setup.listeners) do pcall(f, op, filename) end
  end)
end

local function setupsFolder()
  return ac.getFolder(ac.FolderID.UserSetups) .. "\\" .. ac.getCarID(0)
end

function setup.exists(trackID, stem)
  return io.exists(setupsFolder() .. "\\" .. trackID .. "\\" .. stem .. ".ini")
end

-- put a description into the saved file's [ABOUT] section (shown in the setup list)
local function describe(path, text)
  local body = io.load(path)
  if not body or text == nil or text == "" then return false end
  text = text:gsub("[\r\n]", " "):sub(1, 200)
  local new, n = body:gsub("(%[ABOUT%][^%[]-DESCRIPTION=)[^\r\n]*", "%1" .. text:gsub("%%", "%%%%"), 1)
  if n == 0 then
    if body:find("%[ABOUT%]") then
      new = body:gsub("%[ABOUT%]\r?\n", "[ABOUT]\nDESCRIPTION=" .. text:gsub("%%", "%%%%") .. "\n", 1)
    else
      new = "[ABOUT]\nDESCRIPTION=" .. text .. "\n\n" .. body
    end
  end
  return io.save(path, new)
end

---Apply changes to the car, verify each, save as a new setup file.
---@param changes {name: string, to: integer}[]
---@return {ok: boolean, stem: string|nil, failed: table[], message: string, state: string|nil}
function setup.applyAndSave(changes, trackID, stem, description)
  if not setup.editable() then
    return { ok = false, failed = {}, message = "Open the setup screen in the pits to apply." }
  end
  local failed = {}
  for _, ch in ipairs(changes) do
    local ok, res = pcall(ac.setSetupSpinnerValue, ch.name, ch.to)
    if not ok or res == false then failed[#failed + 1] = { name = ch.name, why = "rejected" } end
  end
  -- read back every value
  for _, ch in ipairs(changes) do
    local ok, got = pcall(ac.getSetupSpinnerValue, ch.name, -999999)
    if not ok or got ~= ch.to then
      failed[#failed + 1] = { name = ch.name, why = "reads " .. tostring(got) .. ", wanted " .. ch.to }
    end
  end
  local state
  if has("getCarSetupState") then
    local okS, s = pcall(ac.getCarSetupState)
    if okS then state = s end
  end
  if #failed > 0 then
    -- put everything back so the car isn't left half-changed
    local stuck = 0
    for _, ch in ipairs(changes) do
      if ch.from ~= nil then
        pcall(ac.setSetupSpinnerValue, ch.name, ch.from)
        local ok, got = pcall(ac.getSetupSpinnerValue, ch.name, -999999)
        if not ok or got ~= ch.from then stuck = stuck + 1 end
      end
    end
    return { ok = false, failed = failed, state = state,
      message = #failed .. " value(s) didn't take, so nothing was saved and the other changes were undone"
        .. (stuck > 0 and string.format(" (%d couldn't be put back: check the setup)", stuck) or "") .. "." }
  end
  local rel = trackID .. "\\" .. stem .. ".ini"
  local okSave, saved = pcall(ac.saveCurrentSetup, rel)
  if not okSave or not saved then
    return { ok = false, failed = {}, state = state, message = "Values applied, but saving " .. stem .. " failed." }
  end
  local full = setupsFolder() .. "\\" .. rel
  pcall(describe, full, description)
  setup.file = full
  if has("setActiveSetupName") then pcall(ac.setActiveSetupName, stem, trackID, false) end
  if has("refreshSetups") then pcall(ac.refreshSetups) end
  return { ok = true, stem = stem, failed = {}, state = state, path = full,
    message = "Checked " .. #changes .. " of " .. #changes .. " values, saved as " .. stem .. "." }
end

---Load a setup file of this track (setup screen only; used to roll back).
function setup.load(trackID, stem)
  if not has("loadSetup") or not setup.exists(trackID, stem) then return false end
  local path = setupsFolder() .. "\\" .. trackID .. "\\" .. stem .. ".ini"
  local ok, res = pcall(ac.loadSetup, path)
  if ok and res then
    setup.file = path
    setup.cat = nil   -- values changed
    return true
  end
  return false
end

---Save the current setup as base.ini for this track (only if it doesn't exist yet).
function setup.saveBase(trackID)
  if setup.exists(trackID, "base") then return false, "base.ini already exists for this track." end
  local ok, saved = pcall(ac.saveCurrentSetup, trackID .. "\\base.ini")
  if ok and saved then
    setup.file = setupsFolder() .. "\\" .. trackID .. "\\base.ini"
    return true, "Saved as base.ini."
  end
  return false, "Saving base.ini failed."
end

return setup
