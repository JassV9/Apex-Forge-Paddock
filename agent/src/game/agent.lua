-- The in-game agent: samples the car, records one pass per corner zone and writes pass records
-- to the spool for core to ingest. It records facts only: it never judges a setup and never
-- calls an LLM. All CSP calls of the capture path live in this file and in carlink.lua.

local carlink = require("game.carlink")
local corners = require("core.corners")
local ghost = require("game.ghost")
local lines = require("core.spool_lines")
local plan = require("game.plan")
local recorder = require("core.recorder")
local setup = require("game.setup")
local spool = require("core.spool")

local agent = {
  VERSION = "0.1.0",
  MAP_VERSION = 1,  -- bump when corners.lua changes what it detects
  ready = false,
  status = "starting",
  source = "human",  -- who drives the current spool session
  passes = 0,       -- passes recorded this session
  writes = 0,       -- chunk files written
}

local function safeName(text) return (tostring(text or ""):gsub("[^%w%._%-]", "_")) end

local function nowMs() return (agent.wallStart + (os.preciseClock() - agent.clockStart)) * 1000 end

local function buildMap(trackLength)
  if ac.hasTrackSpline and not ac.hasTrackSpline() then return nil end
  local count = math.max(300, math.min(3000, math.floor(trackLength / 4)))
  local points = {}
  for i = 0, count - 1 do
    local p = ac.trackProgressToWorldCoordinate(i / count, true)
    points[#points + 1] = { x = p.x, z = p.z }
  end
  local ok, map = pcall(corners.build, points, trackLength)
  return ok and map or nil
end

-- The setup can only change in the pits, so the hash is refreshed at start and on pit exit.
local function refreshSetupHash()
  local ok, key, values = pcall(function()
    if not setup.cat and not setup.scan(nil) then return nil end
    local current = setup.current()
    return setup.key(current), current
  end)
  local hash = ok and key ~= "unknown" and key or nil
  if hash and not agent.catalogSent then
    agent.out:add(lines.catalog(agent.sessionId, setup.cat))
    agent.catalogSent = true
  end
  if hash and hash ~= agent.setupHash then
    agent.out:add(lines.setup(agent.sessionId, hash, values))
  end
  agent.setupHash = hash
end

local function sessionMeta()
  return {
    sessionId = agent.sessionId, startedAtMs = nowMs(), source = agent.source,
    agentVersion = agent.VERSION, carId = ac.getCarID(0), carName = ac.getCarName(0),
    trackId = ac.getTrackID(), layout = ac.getTrackLayout() or "", trackName = ac.getTrackName(),
    mapVersion = agent.MAP_VERSION,
  }
end

function agent.init()
  agent.wallStart, agent.clockStart = os.time(), os.preciseClock()
  agent.trackLength = ac.getSim().trackLengthM or 0
  agent.map = agent.trackLength > 100 and buildMap(agent.trackLength) or nil
  if not agent.map or #agent.map.list == 0 then
    agent.status = "no corner map: this track has no usable AI line"
    return
  end
  agent.rec = recorder.new(agent.map)
  agent.t = 0
  agent.lastReset = ac.getCar(0).resetCounter
  agent.wasInPit = ac.getCar(0).isInPitlane
  agent.startSession("human")
  if ac.onSessionStart then
    ac.onSessionStart(function()
      agent.rec:reset("session restart")
      plan.pit()
      ghost.reset()
    end)
  end
  plan.poll(agent.t)
  agent.ready = true
  agent.status = string.format("recording %d corners", #agent.map.list)
end

---Begin a spool session. A human's laps and the AI's laps never share a session, so the
---driver of every pass is known (source "human" or "ai").
function agent.startSession(source)
  if agent.out then agent.shutdown() end
  agent.source = source
  local suffix = source == "human" and "" or ("-" .. source)
  agent.sessionId = os.date("%Y%m%dT%H%M%S") .. "-" .. safeName(ac.getCarID(0)) .. suffix
  agent.dir = ac.getFolder(ac.FolderID.ACDocuments) .. "\\paddock\\spool\\" .. agent.sessionId
  io.createDir(agent.dir)
  agent.rec:reset("new session")
  agent.out = spool.new()
  agent.out:add(lines.session(sessionMeta(), agent.map))
  agent.catalogSent, agent.setupHash = false, nil
  refreshSetupHash()
end

-- Every file the agent writes shares one budget: at most one write per WRITE_GAP_S.
local WRITE_GAP_S, lastWriteAt = 0.2, -math.huge
local function mayWrite() return os.preciseClock() - lastWriteAt >= WRITE_GAP_S end

local function write(force)
  if not force and not mayWrite() then return end
  local name, text = agent.out:take(os.preciseClock(), force)
  if not name then return end
  lastWriteAt = os.preciseClock()
  agent.writes = agent.writes + 1
  local path = agent.dir .. "\\" .. name
  if force then
    io.save(path, text, true)  -- shutdown: an async write may never finish
  else
    io.saveAsync(path, text, nil, true)
  end
end

-- What the agent is doing, for tools outside the game (and for checking a run without
-- seeing the screen). Written when it changes, at most once a second.
local lastStatus, nextStatus = nil, 0
local function writeStatus()
  if agent.t < nextStatus or not mayWrite() then return end
  nextStatus = agent.t + 1
  local text = JSON.stringify({
    session_id = agent.sessionId, source = agent.source, passes = agent.passes,
    plan = plan.status(), plan_message = plan.message, ghost = ghost.phase(),
    ghost_message = ghost.message(), ghost_allowed = ghost.allowed(),
    calibration = ghost.calibration(),
  })
  if text == lastStatus then return end
  lastStatus, lastWriteAt = text, os.preciseClock()
  io.saveAsync(ac.getFolder(ac.FolderID.ACDocuments) .. "\\paddock\\agent_status.json", text, nil, true)
end

-- One column per frame; if a second pass finishes first, the pending one is completed at once
-- because the recorder is about to reuse its buffer.
local function encode(raw, car)
  if agent.job then agent.out:add(lines.finish(agent.job)) end
  agent.passes = agent.passes + 1
  agent.job = lines.beginPass(raw, {
    sessionId = agent.sessionId, seq = agent.passes,
    startMs = nowMs() - (raw.tOut - raw.tIn) * 1000,
    lap = car.lapCount, setupHash = agent.setupHash, block = plan.tag(),
    conditions = carlink.conditions(car, ac.getSim()),
  })
end

local function stepEncoding()
  if not agent.job then return end
  local line = lines.step(agent.job)
  if line then
    agent.out:add(line)
    agent.job = nil
  end
end

function agent.update(dt)
  local sim = ac.getSim()
  if sim.isReplayActive then
    agent.rec:reset("replay")
    return
  end
  if sim.isPaused or dt <= 0 then
    write(false)  -- a paused game will not notice the write
    return
  end
  agent.t = agent.t + dt
  local car = ac.getCar(0)
  if car.resetCounter ~= agent.lastReset then
    agent.lastReset = car.resetCounter
    agent.rec:reset("car reset")
  end
  local inPit = car.isInPitlane
  -- a human only picks up a plan in the pits; the AI never stops, so it reads it on track
  if inPit or ghost.phase() ~= "off" then plan.poll(agent.t) end
  if inPit then
    plan.pit()
  elseif agent.wasInPit then
    refreshSetupHash()
    plan.leftPit()
  end
  agent.wasInPit = inPit

  ghost.pollCommands(agent.t)
  ghost.update(dt, car)
  local source = ghost.active() and "ai" or "human"
  if source ~= agent.source then agent.startSession(source) end
  local driving = (not inPit) and (not car.isAIControlled or ghost.active())
  if driving then plan.position(car.splinePosition) end
  local raw = agent.rec:feed(carlink.read(car, agent.trackLength, agent.t), driving)
  if raw then encode(raw, car) end
  stepEncoding()
  write(false)
  writeStatus()
end

function agent.shutdown()
  if not agent.out then return end
  if agent.job then
    agent.out:add(lines.finish(agent.job))
    agent.job = nil
  end
  write(true)
end

return agent
