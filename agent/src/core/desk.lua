-- The setup desk in the game window: what core's API says, turned into what the driver reads
-- and the requests the window sends (docs/adr/0015-in-game-setup-desk.md). Capture still goes
-- through spool files; only these few control calls use HTTP. Pure Lua: no CSP calls.

local desk = {}

---Core's id of the spool session the agent is writing, or nil until core has ingested it.
---@param sessions table[] GET /sessions
function desk.sessionId(sessions, spoolId)
  if type(sessions) ~= "table" then return nil end
  for _, s in ipairs(sessions) do
    if s.spool_id == spoolId then return s.id end
  end
  return nil
end

local function signed(n) return (n > 0 and "+" or "") .. tostring(n) end

---The ranked proposals of GET /sessions/{id}/proposals, as lines for the window.
function desk.proposals(decoded)
  local list = {}
  if type(decoded) ~= "table" or type(decoded.ranked) ~= "table" then return list end
  for i, p in ipairs(decoded.ranked) do
    list[i] = {
      parameter = p.parameter, clicks = p.clicks, why = p.why or "",
      hypothesis = type(p.hypotheses) == "table" and p.hypotheses[1] or nil,
      gainLow = p.gain_low or 0, gainHigh = p.gain_high or 0,
      title = string.format("%s %s click%s", p.label ~= "" and p.label or p.parameter,
        signed(p.clicks), math.abs(p.clicks) == 1 and "" or "s"),
      detail = string.format("%s. About %d laps.", p.why or "", p.laps_needed or 0),
    }
  end
  return list
end

---The body of POST /experiments for a proposal. The prediction is on corner time and only
---when the ranking expects a gain, as in the web form.
function desk.createBody(sessionId, proposal, driver)
  local body = {
    session_id = sessionId, parameter = proposal.parameter, clicks = proposal.clicks,
    statement = "Test: " .. proposal.why .. ".", driver = driver, predictions = {},
  }
  if proposal.gainLow > 0 and proposal.gainHigh >= proposal.gainLow then
    body.predictions[1] = { channel = "corner_time_s", direction = "decrease",
      low = proposal.gainLow, high = proposal.gainHigh }
  end
  if proposal.hypothesis then body.hypothesis_key = proposal.hypothesis end
  return body
end

local OPEN = { proposed = true, planned = true, running = true, evaluating = true,
  tradeoff = true, inconclusive = true }

---The experiments of this car that still need something, newest first, with the laps driven.
---@param experiments table[] GET /experiments
function desk.openExperiments(experiments, carName)
  local list = {}
  if type(experiments) ~= "table" then return list end
  for _, e in ipairs(experiments) do
    if e.car_name == carName and OPEN[e.state] then
      local done, target = 0, 0
      for _, b in ipairs(e.blocks or {}) do
        done, target = done + math.min(b.laps_done or 0, b.laps_target or 0), target + (b.laps_target or 0)
      end
      list[#list + 1] = { id = e.id, state = e.state, driver = e.driver or "human",
        title = string.format("#%d %s %s", e.id, e.parameter_label ~= "" and e.parameter_label or e.parameter,
          signed(e.clicks)),
        lapsDone = done, lapsTarget = target, readyToJudge = e.state == "running" and target > 0 and done >= target }
    end
  end
  table.sort(list, function(a, b) return a.id > b.id end)
  return list
end

---The last verdict of this car, for the window: decision and the reason in one line.
function desk.lastVerdict(experiments, carName)
  if type(experiments) ~= "table" then return nil end
  local best
  for _, e in ipairs(experiments) do
    if e.car_name == carName and type(e.verdict) == "table" and (not best or e.id > best.id) then best = e end
  end
  if not best then return nil end
  return string.format("#%d %s: %s", best.id, best.state, best.verdict.reason or "")
end

---This car's open AI training program (GET /training), or nil: its id, what it is doing, and
---how far it is (experiments started of its target).
function desk.training(programs, carName)
  if type(programs) ~= "table" then return nil end
  for _, p in ipairs(programs) do
    if p.car_name == carName and (p.state == "running" or p.state == "waiting") then
      return { id = p.id, state = p.state, message = p.message or "",
        done = type(p.steps) == "table" and #p.steps or 0, target = p.target or 0 }
    end
  end
  return nil
end

return desk
