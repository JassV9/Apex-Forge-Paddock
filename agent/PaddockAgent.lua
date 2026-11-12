-- Apex Forge: Paddock agent. Records every corner pass with its conditions and hands it to the
-- Paddock backend through spool files. Install with tools/install_agent.py.

package.add("src")

local agent = require("game.agent")
local plan = require("game.plan")

local ok, err = pcall(agent.init)
if not ok then
  agent.ready = false
  agent.status = "startup failed: " .. tostring(err)
  ac.log("Paddock agent startup failed: " .. tostring(err))
end

local errors = 0
function script.update(dt)
  if not agent.ready then return end
  local fine, problem = pcall(agent.update, dt)
  if not fine then
    errors = errors + 1
    agent.status = "error: " .. tostring(problem)
    if errors <= 5 then ac.log("Paddock agent error: " .. tostring(problem)) end
  end
end

function script.windowMain()
  ui.text("Paddock agent " .. agent.VERSION)
  ui.text(agent.status)
  if not agent.ready then return end
  ui.text(string.format("%d passes recorded, %d spool files written", agent.passes, agent.writes))
  ui.separator()
  -- the run plan: code names only, so the driver does not know which setup is the baseline
  ui.text(plan.status())
  if plan.canLoad() and ui.button("Load " .. plan.label()) then plan.loadBlock() end
  if plan.message ~= "" then ui.text(plan.message) end
end

ac.onRelease(function() pcall(agent.shutdown) end)
