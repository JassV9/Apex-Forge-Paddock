-- The run plan of an experiment as the agent follows it (contracts/runplan.schema.json): which
-- block is being driven, how many push laps of it are done, and how each pass is tagged.
--
-- A block is one stint on one setup, known to the driver only by a code name. A stint is an
-- out-lap (lap 0, never compared) and then the block's push laps. A lap counts when the car
-- crosses the start line after having been through the middle of the lap, so a spin on the
-- line, a teleport to the pits or a pit exit just before the line is not a lap.
-- Pure Lua: no CSP calls.

local runplan = {}

local EPSILON = 1e-6

---Check a decoded runplan.json and turn it into the agent's form.
---@param decoded table|nil
---@return table|nil plan, string|nil problem
function runplan.read(decoded)
  if type(decoded) ~= "table" or decoded.v ~= 1 then return nil, "not a version 1 run plan" end
  if type(decoded.experiment_id) ~= "number" or type(decoded.revision) ~= "number" then
    return nil, "run plan has no experiment id or revision"
  end
  if type(decoded.blocks) ~= "table" or #decoded.blocks == 0 then return nil, "run plan has no blocks" end
  local blocks = {}
  for i, b in ipairs(decoded.blocks) do
    if type(b.order) ~= "number" or type(b.label) ~= "string" or type(b.laps_target) ~= "number"
        or type(b.values) ~= "table" then
      return nil, "block " .. i .. " is incomplete"
    end
    blocks[i] = { order = b.order, label = b.label, lapsTarget = b.laps_target, values = b.values }
  end
  table.sort(blocks, function(a, b) return a.order < b.order end)
  return { experimentId = decoded.experiment_id, revision = decoded.revision, blocks = blocks }
end

function runplan.new()
  return { plan = nil, order = nil, pushDone = 0, outLap = true, seenMid = false, onCar = false }
end

local function startBlock(state, order, pushDone)
  state.order, state.pushDone = order, pushDone or 0
  state.outLap, state.seenMid, state.onCar, state.lastPos = true, false, false, nil
end

---The block being driven, or nil when the plan is finished (or there is none).
function runplan.block(state)
  if not state.plan then return nil end
  for _, b in ipairs(state.plan.blocks) do
    if b.order == state.order then return b end
  end
  return nil
end

local function advance(state)
  for _, b in ipairs(state.plan.blocks) do
    if b.order > state.order then return startBlock(state, b.order) end
  end
  startBlock(state, state.order + 1)  -- past the last block: finished
end

---Take over a plan. A new revision of the same experiment keeps the position (blocks are only
---ever appended); another experiment starts at its first block. `saved` is the position the
---agent wrote down before the game was last closed.
---@param saved table|nil { experiment_id, order, push_done }
function runplan.load(state, plan, saved)
  local same = state.plan and state.plan.experimentId == plan.experimentId
  state.plan = plan
  if same then return end
  if saved and saved.experiment_id == plan.experimentId and type(saved.order) == "number" then
    startBlock(state, saved.order, saved.push_done)
  else
    startBlock(state, plan.blocks[1].order)
  end
end

---What to write down so the position survives closing the game.
function runplan.snapshot(state)
  if not state.plan then return nil end
  return { experiment_id = state.plan.experimentId, order = state.order, push_done = state.pushDone }
end

---The setup items that differ from the block's setup, by name.
---@param current table name -> value on the car
---@return {name: string, from: number, to: number}[]
function runplan.changes(block, current)
  local list = {}
  for name, to in pairs(block.values) do
    local from = current[name]
    if from ~= nil and math.abs(from - to) > EPSILON then
      list[#list + 1] = { name = name, from = from, to = to }
    end
  end
  table.sort(list, function(a, b) return a.name < b.name end)
  return list
end

---The car's setup was read (in the pits after loading a block, and on every pit exit).
---Passes are only tagged while the block's setup is what is on the car. A block with no laps
---to drive only loads a setup, so it is done as soon as that setup is on the car.
function runplan.setupIs(state, current)
  local block = runplan.block(state)
  if not block then return end
  state.onCar = #runplan.changes(block, current) == 0
  if state.onCar and block.lapsTarget == 0 then advance(state) end
end

---The car is in the pit lane, or the session restarted: the next lap is an out-lap again.
function runplan.pit(state)
  state.outLap, state.seenMid, state.lastPos = true, false, nil
end

---Feed the car's position on track (0..1 along the lap) while it is driving.
---@return string|nil "lap" when a push lap was completed, "done" when that completed the block
function runplan.position(state, pos)
  local block = runplan.block(state)
  if not block or not state.onCar then return nil end
  local last = state.lastPos
  state.lastPos = pos
  if pos > 0.4 and pos < 0.6 then state.seenMid = true end
  if not (last and last > 0.75 and pos < 0.25 and state.seenMid) then return nil end
  state.seenMid = false
  if state.outLap then
    state.outLap = false
    return nil
  end
  state.pushDone = state.pushDone + 1
  if state.pushDone < block.lapsTarget then return "lap" end
  advance(state)
  return "done"
end

---The tag of a pass that just ended, or nil when it does not belong to a block.
function runplan.tag(state)
  local block = runplan.block(state)
  if not block or not state.onCar then return nil end
  return {
    experimentId = state.plan.experimentId, order = block.order,
    lap = state.outLap and 0 or state.pushDone + 1,
  }
end

---One line for the driver. It names code names only, never which setup is the baseline.
function runplan.status(state)
  if not state.plan then return "No run plan." end
  local block = runplan.block(state)
  if not block then return string.format("Experiment %d: run plan complete.", state.plan.experimentId) end
  local where = string.format("Experiment %d, block %d of %d", state.plan.experimentId, block.order,
    state.plan.blocks[#state.plan.blocks].order)
  if not state.onCar then return where .. ": pit and load " .. block.label .. "." end
  if state.outLap then return string.format("%s: %s, out-lap.", where, block.label) end
  return string.format("%s: %s, push lap %d of %d.", where, block.label, state.pushDone + 1, block.lapsTarget)
end

return runplan
