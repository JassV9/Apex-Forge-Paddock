-- The agent's window: the setup desk. "Run" is what happens on the car now (recording, the
-- block being driven, the AI driver); "Setups" is what to test next and the experiments of
-- this car, started and judged from the game. Telemetry is read in the web dashboard.

local ghost = require("game.ghost")
local link = require("game.desk_link")
local plan = require("game.plan")

local window = {}

local GREEN, AMBER, RED, GREY = rgbm(0.35, 0.85, 0.45, 1), rgbm(1, 0.72, 0.2, 1), rgbm(1, 0.4, 0.35, 1),
  rgbm(0.65, 0.65, 0.7, 1)

local STATE_COLOR = { running = GREEN, evaluating = AMBER, inconclusive = AMBER, tradeoff = AMBER,
  proposed = GREY, planned = GREY }

local function recording(agent)
  ui.textColored(agent.ready and "REC" or "OFF", agent.ready and RED or GREY)
  ui.sameLine()
  ui.text(string.format("%s  |  %d passes  |  %s", agent.status, agent.passes,
    agent.source == "ai" and "AI driving" or "you driving"))
end

local function block()
  ui.header("Run plan")
  ui.textWrapped(plan.status())
  local done, target = plan.progress()
  if target and target > 0 then ui.progressBar(done / target, vec2(-1, 0), done .. " of " .. target .. " push laps") end
  if plan.canLoad() and ui.button("Load " .. plan.label()) then plan.loadBlock() end
  if plan.message ~= "" then ui.textColored(plan.message, GREY) end
end

local function aiDriver()
  ui.header("AI driver")
  if ghost.message() ~= "" then ui.textWrapped(ghost.message()) end
  if ghost.allowed() then ui.textColored(ghost.calibration(), GREY) end
  if not ghost.allowed() then
    ui.textColored("Not on this track: it gives apps no physics access (see tools/prepare_test_layout.py).", GREY)
  elseif ghost.active() then
    if ui.button("Stop the AI driver") then ghost.stop() end
  elseif plan.blockOrder() then
    if ui.button("Let the AI drive this plan") then ghost.resume() end
  elseif ui.button("Let the AI drive laps") then
    ghost.lap()
  end
end

local function experiments()
  ui.header("Experiments on this car")
  if #link.experiments == 0 then ui.textColored("None open.", GREY) end
  for _, e in ipairs(link.experiments) do
    ui.textColored(e.state, STATE_COLOR[e.state] or GREY)
    ui.sameLine()
    ui.text(e.title .. (e.driver == "ai" and "  (AI)" or ""))
    if e.lapsTarget > 0 then ui.progressBar(e.lapsDone / e.lapsTarget, vec2(-1, 0), e.lapsDone .. "/" .. e.lapsTarget .. " laps") end
    if e.readyToJudge and ui.button("Judge #" .. e.id) then link.act(e.id, "evaluate") end
    if e.state == "running" then
      if e.readyToJudge then ui.sameLine() end
      if ui.button("Abort #" .. e.id) then link.act(e.id, "abort") end
    end
  end
  if link.verdict then ui.textWrapped("Last verdict: " .. link.verdict) end
end

local function busyOnCar()
  for _, e in ipairs(link.experiments) do
    if e.state == "running" or e.state == "evaluating" then return true end
  end
  return false
end

local function proposals()
  ui.header("What to test next")
  if not link.sessionId then return ui.textColored("Core has not seen this session yet.", GREY) end
  if #link.proposals == 0 then
    ui.textColored("No proposals: this session has no diagnosis yet.", GREY)
    if ui.button("Diagnose this session") then link.diagnose() end
    return
  end
  local blocked = busyOnCar()
  for i, p in ipairs(link.proposals) do
    if i > 5 then break end
    ui.text(p.title)
    ui.textColored(p.detail, GREY)
    if not blocked and ui.button("I drive it##" .. i) then link.test(p, "human") end
    if not blocked and ghost.allowed() then
      ui.sameLine()
      if ui.button("AI drives it##" .. i) then link.test(p, "ai") end
    end
  end
  if blocked then ui.textColored("Finish or judge the running experiment first.", AMBER) end
end

---Draw the window. `agent` is game/agent.lua.
function window.draw(agent)
  ui.pushFont(ui.Font.Title)
  ui.text("Paddock")
  ui.popFont()
  recording(agent)
  if not agent.ready then return end
  link.refresh(agent.t or 0, agent.sessionId, ac.getCarName(0))
  ui.tabBar("paddock", function()
    ui.tabItem("Run", function() block() aiDriver() end)
    ui.tabItem("Setups", function()
      if link.message ~= "" then ui.textColored(link.message, link.reachable == false and RED or GREY) end
      experiments()
      proposals()
    end)
  end)
end

return window
