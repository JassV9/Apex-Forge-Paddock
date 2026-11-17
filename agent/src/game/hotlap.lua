-- The hotlap window (docs/adr/0019-ai-reference-driver.md): the last corner pass for six
-- seconds (time, gap to your best on this setup, gap to the AI on this setup, what the samples
-- showed) and, on a toggle, the corners table. Only while you drive: nothing while the AI has
-- the car. The logic is core/hotlap.lua; this file draws it and feeds it.

local hotlap = require("core.hotlap")
local link = require("game.hotlap_link")

local hud = { state = hotlap.new(), showTable = false }

local WHITE, GREY = { 1, 1, 1 }, { 0.65, 0.65, 0.7 }
local TONE = { driver = { 1, 0.4, 0.35 }, ahead = { 0.35, 0.85, 0.45 }, even = { 0.65, 0.65, 0.7 } }

local function ink(rgb, alpha) return rgbm(rgb[1], rgb[2], rgb[3], alpha) end

---A finished pass of the human. Called by game/agent.lua once per pass; `raw` is the
---recorder's pass, readable until the next one finishes.
function hud.pass(raw, setupHash, now)
  hud.state:pass(raw.corner.id, raw.corner.name, raw.tOut - raw.tIn, setupHash,
    hotlap.verdict(raw), now)
end

local function lastPass(now)
  local alpha, last = hud.state:alpha(now), hud.state.last
  if alpha <= 0 then
    return ui.textColored("Drive a corner: its time and the gap to the AI show here.", ink(GREY, 1))
  end
  ui.pushFont(ui.Font.Title)
  ui.textColored(string.format("%s  %s", last.name, hotlap.seconds(last.time)), ink(WHITE, alpha))
  ui.popFont()
  local tone = TONE[hotlap.against(last.deltaAI, last.margin) or "even"]
  ui.textColored("vs best " .. (last.deltaBest and hotlap.signed(last.deltaBest) or "first"),
    ink(GREY, alpha))
  ui.sameLine()
  ui.textColored("  vs AI " .. hotlap.signed(last.deltaAI), ink(tone, alpha))
  if last.verdict ~= "" then ui.textColored(last.verdict, ink(TONE.driver, alpha)) end
end

local function cornersTable(setupHash)
  local rows = hud.state:rows(setupHash)
  if #rows == 0 then return ui.textColored("No corner on this setup yet.", ink(GREY, 1)) end
  ui.pushFont(ui.Font.Monospace)
  ui.text(string.format("%-6s %8s %8s %8s", "", "best", "AI", "delta"))
  for _, row in ipairs(rows) do
    local tone = TONE[hotlap.against(row.delta, row.margin) or "even"]
    ui.textColored(string.format("%-6s %8s %8s %8s", row.name, hotlap.seconds(row.best),
      hotlap.seconds(row.ai), hotlap.signed(row.delta)), ink(tone, 1))
  end
  ui.popFont()
end

---Draw the window. `agent` is game/agent.lua.
function hud.draw(agent)
  if not agent.ready or agent.source ~= "human" or ac.getCar(0).isAIControlled then return end
  local ids = { car = ac.getCarID(0), track = ac.getTrackID(), layout = ac.getTrackLayout() or "" }
  link.refresh(agent.t or 0, hud.state, ids, agent.setupHash)
  lastPass(agent.t or 0)
  if ui.checkbox("Corners table", hud.showTable) then hud.showTable = not hud.showTable end
  if hud.showTable then cornersTable(agent.setupHash) end
  if link.message ~= "" then ui.textColored(link.message, ink(GREY, 1)) end
end

return hud
