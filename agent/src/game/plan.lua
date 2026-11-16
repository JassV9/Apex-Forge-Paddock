-- The run plan in the game: reads this car's plan (outbox/runplans/<car>.json, written by
-- core, one file per car so an experiment waiting on another car is left alone), loads a block's
-- setup when the driver asks for it, follows the car through the blocks and says how each
-- pass is tagged. The rules live in core/runplan.lua; this file is the CSP side of them.

local runplan = require("core.runplan")
local setup = require("game.setup")

local plan = {
  state = runplan.new(),
  message = "",        -- what happened last, for the window
  POLL_S = 2,          -- how often the outbox is looked at, and only while in the pits
}

local lastText, lastSaved, nextPoll = nil, nil, 0

local function folder() return ac.getFolder(ac.FolderID.ACDocuments) .. "\\paddock" end

-- the car's id as core names its files (setups.SafeName)
local function carFile()
  local name = tostring(ac.getCarID(0) or ""):gsub("[^%w%._%-]+", "_"):gsub("^[%._]+", ""):gsub("[%._]+$", "")
  return name ~= "" and name or "setup"
end

-- where the position in an experiment is kept; agent_state.json is the older, single file
local function statePath(id) return folder() .. "\\state\\e" .. tostring(id) .. ".json" end

local function decode(text)
  local ok, decoded = pcall(JSON.parse, text)
  return ok and decoded or nil
end

-- the position is written down when it changes, so it survives closing the game
local function remember()
  local snapshot = runplan.snapshot(plan.state)
  if not snapshot then return end
  local text = JSON.stringify(snapshot)
  if text == lastSaved then return end
  lastSaved = text
  io.createDir(folder() .. "\\state")
  io.saveAsync(statePath(snapshot.experiment_id), text, nil, true)
end

---Look for a new run plan. Reading is cheap but not free, so it happens every POLL_S seconds
---and only when the caller says the car is in the pits (or at startup).
function plan.poll(now)
  if now < nextPoll then return end
  nextPoll = now + plan.POLL_S
  local text = io.load(folder() .. "\\outbox\\runplans\\" .. carFile() .. ".json", nil)
  if not text or text == lastText then return end
  lastText = text
  local loaded, problem = runplan.read(decode(text))
  if not loaded then
    plan.message = "Run plan ignored: " .. problem
    return
  end
  local saved = decode(io.load(statePath(loaded.experimentId), "") or "")
    or decode(io.load(folder() .. "\\agent_state.json", "") or "")
  runplan.load(plan.state, loaded, saved)
  plan.message = string.format("Run plan of experiment %d loaded.", loaded.experimentId)
  remember()
end

---The car's setup as the setup menu has it, or nil when the menu cannot be read.
local function currentSetup()
  if not setup.cat and not setup.scan(nil) then return nil end
  return setup.current()
end

---Put the current block's setup on the car (setup screen in the pits only).
function plan.loadBlock()
  local block = runplan.block(plan.state)
  local current = currentSetup()
  if not block or not current then
    plan.message = "Nothing to load."
    return false
  end
  local result = setup.apply(runplan.changes(block, current))
  plan.message = block.label .. ": " .. result.message
  if result.ok then
    runplan.setupIs(plan.state, setup.current())
    remember()
  end
  return result.ok
end

---The car left the pits: whatever is on the car now decides whether passes are tagged.
function plan.leftPit()
  local current = currentSetup()
  if current then runplan.setupIs(plan.state, current) end
  remember()
end

---The car is in the pit lane, or the session restarted.
function plan.pit() runplan.pit(plan.state) end

---Every frame on track: the car's position along the lap (0..1).
function plan.position(pos)
  if runplan.position(plan.state, pos) then remember() end
end

function plan.tag() return runplan.tag(plan.state) end

---Whether the window should offer to load the current block's setup.
function plan.canLoad()
  local block = runplan.block(plan.state)
  return block ~= nil and not plan.state.onCar and setup.editable()
end

function plan.label()
  local block = runplan.block(plan.state)
  return block and block.label or ""
end

function plan.status() return runplan.status(plan.state) end

---Who the plan says drives its laps: "human" or "ai" (nil without a plan).
function plan.driver() return plan.state.plan and plan.state.plan.driver or nil end

function plan.experimentId() return plan.state.plan and plan.state.plan.experimentId or nil end

---Push laps done and to do in the block being driven (nil without a block).
function plan.progress()
  local block = runplan.block(plan.state)
  if not block then return nil, nil end
  return plan.state.pushDone, block.lapsTarget
end

---Laps counted in the block being driven, the out-lap included (nil without a block).
function plan.lapsDone()
  if not runplan.block(plan.state) then return nil end
  return plan.state.pushDone + (plan.state.outLap and 0 or 1)
end

---The order of the block being driven, or nil when there is none left.
function plan.blockOrder()
  local block = runplan.block(plan.state)
  return block and block.order or nil
end

return plan
