-- Minimal mock of the CSP Lua app API: enough to run Apex Forge end to end outside the game.
-- Python fills MOCK: points {x,z}[], L, root (temp folder), spinners[], tabs{name->TAB},
-- tyres{FRONT/REAR -> {PRESSURE_IDEAL, FRICTION_LIMIT_ANGLE}}, carID, trackID, layout, and
-- helpers fs_exists(path), fs_mkdir(path), json_stringify(t), json_parse(s).

local M = MOCK

-- vectors / colours ------------------------------------------------------------------------
function vec2(x, y) return { x = x or 0, y = y or x or 0 } end
function vec3(x, y, z) return { x = x or 0, y = y or 0, z = z or 0 } end
rgbm = setmetatable({}, { __call = function(_, r, g, b, m) return { r = r, g = g, b = b, mult = m } end })
rgb = function(r, g, b) return { r = r, g = g, b = b } end
function math.clamp(x, a, b) return math.max(a, math.min(b, x)) end

package.add = function(dir) package.path = M.appRoot .. "/" .. dir .. "/?.lua;" .. package.path end

M.calls = { setSpinner = 0, save = 0, saveAsync = 0, clipboard = 0, log = {} }
M.callbacks = { setupFile = {}, session = {}, release = {} }
M.setup = {}
for _, sp in ipairs(M.spinners) do M.setup[sp.name] = sp.value end
M.editable = false
M.clipboard = ""
M.clicks = {}        -- label substring -> true: next ui.button/smallButton with it returns true
M.texts = 0
M.pushes = { font = 0, color = 0, disabled = 0 }
M.maxDepth = 0

-- car state ----------------------------------------------------------------------------------
local function wheel()
  return {
    slipAngle = 0, slipRatio = 0, ndSlip = 0, load = 3000, tyreInsideTemperature = 85,
    tyreMiddleTemperature = 82, tyreOutsideTemperature = 78, tyreCoreTemperature = 90,
    tyreOptimumTemperature = 90, tyrePressure = 27.5,
  }
end
M.car = {
  index = 0, splinePosition = 0, speedMs = 0, speedKmh = 0, gas = 0, brake = 0, steer = 0,
  acceleration = vec3(), localAngularVelocity = vec3(), wheels = { [0] = wheel(), wheel(), wheel(), wheel() },
  wheelsOutside = 0, rideHeight = { [0] = 0.02, 0.04 }, isInPitlane = false, isInPit = false,
  isAIControlled = false, resetCounter = 0, tractionType = 0, compoundIndex = 0,
}
M.sim = { trackLengthM = M.L, isPaused = false, isReplayActive = false, raceSessionType = 2 }

-- ac -------------------------------------------------------------------------------------------
ac = {}
ac.FolderID = { ACDocuments = 31, UserSetups = 11, ScriptConfig = 40 }
function ac.getFolder(id)
  if id == ac.FolderID.UserSetups then return M.root .. "\\setups" end
  return M.root
end
function ac.getCar(i) return M.car end
function ac.getSim() return M.sim end
function ac.getCarID() return M.carID end
function ac.getCarName() return M.carName or M.carID end
function ac.getTrackID() return M.trackID end
function ac.getTrackLayout() return M.layout end
function ac.getTrackName() return M.trackName or M.trackID end
function ac.hasTrackSpline() return true end
function ac.log(s) M.calls.log[#M.calls.log + 1] = tostring(s) end
function ac.storage(t) return t end
function ac.trackProgressToWorldCoordinate(p, linear)
  local n = #M.points
  local f = (p % 1) * n
  local i = math.floor(f)
  local a, b = M.points[i % n + 1], M.points[(i + 1) % n + 1]
  local t = f - i
  return vec3(a.x + (b.x - a.x) * t, 0, a.z + (b.z - a.z) * t)
end
function ac.getSetupSpinners()
  local out = {}
  for i, sp in ipairs(M.spinners) do
    local c = {}
    for k, v in pairs(sp) do c[k] = v end
    c.value = M.setup[sp.name]
    out[i] = c
  end
  return out
end
function ac.getSetupSpinnerValue(name, fb)
  local v = M.setup[name]
  if v == nil then return fb end
  return v
end
local spinnerBy = {}
for _, sp in ipairs(M.spinners) do spinnerBy[sp.name] = sp end
function ac.setSetupSpinnerValue(name, v)
  M.calls.setSpinner = M.calls.setSpinner + 1
  local sp = spinnerBy[name]
  if not M.editable or not sp or v < sp.min or v > sp.max then return false end
  if M.reject and M.reject[name] then return true end   -- pretends to work but doesn't change
  M.setup[name] = v
  return true
end
function ac.isSetupAvailableToEdit() return M.editable end
function ac.getCarSetupState() return "legal" end

local function setupsDir() return M.root .. "\\setups\\" .. M.carID end
local function writeSetup(full)
  local lines = { "[ABOUT]", "AUTHOR=jass", "DESCRIPTION=", "" }
  local names = {}
  for k in pairs(M.setup) do names[#names + 1] = k end
  table.sort(names)
  for _, k in ipairs(names) do
    lines[#lines + 1] = "[" .. k .. "]"
    lines[#lines + 1] = "VALUE=" .. tostring(M.setup[k])
    lines[#lines + 1] = ""
  end
  lines[#lines + 1] = "[CAR]"
  lines[#lines + 1] = "MODEL=" .. M.carID
  local dir = full:match("^(.*)\\[^\\]+$")
  M.fs_mkdir(dir)
  local f = assert(io.open(full, "wb"))
  f:write(table.concat(lines, "\n"))
  f:close()
end
function ac.saveCurrentSetup(rel)
  local full = rel:match("^%a:") and rel or (setupsDir() .. "\\" .. rel)
  writeSetup(full)
  M.lastSaved = full
  for _, cb in ipairs(M.callbacks.setupFile) do cb("save", "C:\\Users\\x\\AppData\\Local\\Temp\\setup_tmp.ini") end
  for _, cb in ipairs(M.callbacks.setupFile) do cb("save", full) end
  return true
end
function ac.loadSetup(path) return M.editable end
function ac.refreshSetups() return true end
function ac.setActiveSetupName() return true end
function ac.onSetupFile(cb) M.callbacks.setupFile[#M.callbacks.setupFile + 1] = cb end
function ac.onSessionStart(cb) M.callbacks.session[#M.callbacks.session + 1] = cb end
function ac.onRelease(cb) M.callbacks.release[#M.callbacks.release + 1] = cb end
function ac.setClipboardText(t)
  M.calls.clipboard = M.calls.clipboard + 1
  M.clipboard = t
  return true
end

ac.INIConfig = {}
function ac.INIConfig.carData(_, file)
  local obj = {}
  function obj.get(self, section, key, default)
    if file == "setup.ini" and key == "TAB" then return M.tabs[section] or default end
    if file == "tyres.ini" then
      local base = section:gsub("_%d+$", "")
      local t = M.tyres and M.tyres[base]
      if t and t[key] then return t[key] end
    end
    return default
  end
  return obj
end
function ac.INIConfig.currentSetup() return { filename = M.currentSetupFile } end

-- io / JSON -------------------------------------------------------------------------------------
function io.exists(p) return M.fs_exists(p) end
function io.createDir(p) M.fs_mkdir(p) return true end
function io.load(p, fb)
  local f = io.open(p, "rb")
  if not f then return fb end
  local s = f:read("*a")
  f:close()
  return s
end
function io.save(p, data, ensure)
  M.calls.save = M.calls.save + 1
  local f = io.open(p, "wb")
  if not f then return false end
  f:write(data)
  f:close()
  return true
end
function io.saveAsync(p, data, cb, ensure)
  M.calls.saveAsync = M.calls.saveAsync + 1
  local f = io.open(p, "wb")
  if f then f:write(data) f:close() end
  if cb then cb(nil) end
end
JSON = { stringify = function(t) return M.json_stringify(t) end, parse = function(s) return M.json_parse(s) end }

-- ui ----------------------------------------------------------------------------------------------
ui = {}
ui.Font = { Small = 1, Tiny = 2, Monospace = 3, Main = 4, Title = 6, Huge = 7 }
ui.StyleColor = { Button = 21, WindowBg = 2 }
ui.InputTextFlags = { Placeholder = 0x400000 }
local function clicked(label)
  for k in pairs(M.clicks) do
    if label:find(k, 1, true) then
      M.clicks[k] = nil
      M.clickedLabels[#M.clickedLabels + 1] = label
      return true
    end
  end
  return false
end
M.clickedLabels = {}
M.disabledDepth = 0
function ui.button(label)
  M.seen[#M.seen + 1] = label
  if M.disabledDepth > 0 then return false end
  return clicked(label)
end
ui.smallButton = ui.button
function ui.checkbox(label, v) return clicked(label) end
function ui.radioButton(label, v) return clicked(label) end
function ui.inputText(label, s) return s, false, false end
function ui.text(s) assert(type(s) == "string", "ui.text needs a string") M.texts = M.texts + 1 end
function ui.textColored(s, c) assert(type(s) == "string", "ui.textColored needs a string") M.texts = M.texts + 1 end
ui.textWrapped = function(s) assert(type(s) == "string") M.texts = M.texts + 1 end
ui.textDisabled = ui.text
ui.bulletText = ui.text
function ui.sameLine() end
function ui.separator() end
function ui.newLine() end
function ui.getCursorX() return 0 end
function ui.availableSpaceX() return 400 end
function ui.setNextItemWidth() end
function ui.pushFont() M.pushes.font = M.pushes.font + 1 end
function ui.popFont() M.pushes.font = M.pushes.font - 1 assert(M.pushes.font >= 0, "popFont underflow") end
function ui.pushStyleColor() M.pushes.color = M.pushes.color + 1 end
function ui.popStyleColor() M.pushes.color = M.pushes.color - 1 assert(M.pushes.color >= 0, "popStyleColor underflow") end
function ui.pushDisabled() M.disabledDepth = M.disabledDepth + 1 end
function ui.popDisabled() M.disabledDepth = M.disabledDepth - 1 assert(M.disabledDepth >= 0, "popDisabled underflow") end
function ui.tabBar(id, fn) fn() end
function ui.tabItem(label, fn) fn() end
function ui.childWindow(id, size, border, flags, fn) fn() end
function ui.treeNode(label, flags, fn) fn() end
function ui.getClipboardText() return M.clipboard end
function ui.combo(label, preview, flags, fn) fn() end
function ui.selectable(label, sel) return clicked(label) end
M.seen = {}

-- web: Python answers (MOCK.web_post(url, headers, body) -> status, body)
web = {}
function web.timeouts() end
function web.post(url, headers, body, cb)
  local status, resp = M.web_post(url, headers, body)
  cb(nil, { status = status, body = resp, headers = {} })
end
function web.get(url, headers, cb)
  local status, resp = M.web_post(url, headers, nil)
  cb(nil, { status = status, body = resp, headers = {} })
end

script = {}
function setInterval(fn, s) end
