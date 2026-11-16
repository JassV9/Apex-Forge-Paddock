-- The setup desk's link to core over HTTP (docs/adr/0015-in-game-setup-desk.md). It asks only
-- while the window is drawn, at most every REFRESH_S, one request at a time (CSP allows two),
-- and sends nothing unless the driver clicks. Capture never depends on it: if core is not
-- running, the desk says so and recording goes on through the spool.

local desk = require("core.desk")

local link = {
  url = "http://127.0.0.1:8080",
  REFRESH_S = 10,
  sessionId = nil, proposals = {}, experiments = {}, verdict = nil,
  message = "", busy = false, reachable = nil,
}

local nextRefresh = 0

local function answer(cb)
  return function(err, res)
    if err or not res or res.status >= 400 then
      local why = err or (res and res.body) or "no answer"
      return cb(nil, tostring(why))
    end
    local ok, decoded = pcall(JSON.parse, res.body or "")
    cb(ok and decoded or {}, nil)
  end
end

local function get(path, cb)
  local ok = pcall(web.get, link.url .. path, nil, answer(cb))
  if not ok then cb(nil, "web requests are not available") end
end

local function post(path, body, cb)
  local headers = { ["Content-Type"] = "application/json" }
  local ok = pcall(web.post, link.url .. path, headers, body and JSON.stringify(body) or "", answer(cb))
  if not ok then cb(nil, "web requests are not available") end
end

local function done(message)
  link.busy = false
  if message then link.message = message end
end

local function loadProposals(cb)
  if not link.sessionId then return cb() end
  get("/sessions/" .. link.sessionId .. "/proposals", function(p)
    link.proposals = desk.proposals(p)
    cb()
  end)
end

---Ask core what it knows about this session and car. Call from the window, every frame it is
---drawn; it does nothing until REFRESH_S has passed.
function link.refresh(now, spoolId, carName, force)
  if link.busy or (now < nextRefresh and not force) then return end
  nextRefresh, link.busy = now + link.REFRESH_S, true
  get("/sessions", function(sessions, err)
    link.reachable = sessions ~= nil
    if not sessions then return done("Core is not answering at " .. link.url .. " (" .. err .. ").") end
    link.sessionId = desk.sessionId(sessions, spoolId)
    get("/experiments", function(experiments)
      link.experiments = desk.openExperiments(experiments, carName)
      link.verdict = desk.lastVerdict(experiments, carName)
      loadProposals(function() done() end)
    end)
  end)
end

local function chain(steps, finished)
  local function run(i, value)
    if i > #steps then return finished(value) end
    steps[i](value, function(next, err)
      if next == nil then return done(err) end
      run(i + 1, next)
    end)
  end
  run(1, nil)
end

---Propose, plan and start the experiment of a proposal, with the human or the AI driving.
function link.test(proposal, driver, onStarted)
  if link.busy or not link.sessionId then return end
  link.busy, link.message = true, "Starting " .. proposal.title .. "..."
  chain({
    function(_, cb) post("/experiments", desk.createBody(link.sessionId, proposal, driver), cb) end,
    function(exp, cb) post("/experiments/" .. exp.id .. "/plan", nil, cb) end,
    function(exp, cb) post("/experiments/" .. exp.id .. "/start", nil, cb) end,
  }, function(exp)
    done(string.format("Experiment %d started: pit for its first block.", exp.id))
    nextRefresh = 0
    if onStarted then onStarted(exp) end
  end)
end

---Move an experiment along: evaluate, extend, revert, abort.
function link.act(id, action)
  if link.busy then return end
  link.busy = true
  post("/experiments/" .. id .. "/" .. action, nil, function(exp, err)
    nextRefresh = 0
    done(exp and string.format("Experiment %d: %s.", id, exp.state) or err)
  end)
end

---Have core diagnose this session, which is what the proposals are made from.
function link.diagnose()
  if link.busy or not link.sessionId then return end
  link.busy = true
  post("/sessions/" .. link.sessionId .. "/diagnosis", nil, function(d, err)
    nextRefresh = 0
    done(d and "Session diagnosed." or ("Not diagnosed yet: " .. tostring(err)))
  end)
end

return link
