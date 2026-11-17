-- The hotlap display's link to core (docs/adr/0019-ai-reference-driver.md): asks GET /reference
-- for the AI's times on the setup on the car. Only while the window is drawn, one request at a
-- time, when the setup changes and otherwise every REFRESH_S. Capture never waits on it: if
-- core is not running the display shows no AI column and recording goes on.

local hotlap = require("core.hotlap")

local link = {
  url = "http://127.0.0.1:8080",
  REFRESH_S = 30,
  busy = false,
  setupHash = nil,   -- the setup last asked for
  message = "",
}

local nextRefresh = 0

local function answered(state, setupHash, layout)
  return function(err, res)
    link.busy = false
    if err or not res or res.status >= 400 then
      link.message = "No AI reference: core did not answer."
      return
    end
    local ok, reply = pcall(JSON.parse, res.body or "")
    if not ok or type(reply) ~= "table" then
      link.message = "No AI reference: core's answer could not be read."
      return
    end
    state:setReference(setupHash, reply)
    local count = #(reply.corners or {})
    link.message = count > 0 and "" or hotlap.noReferenceText(layout)
  end
end

---Ask core for the AI's times on this setup, if it is time to. Call every frame the window is
---drawn; `ids` is { car, track, layout }.
function link.refresh(now, state, ids, setupHash)
  if link.busy or setupHash == nil then return end
  if setupHash == link.setupHash and now < nextRefresh then return end
  link.busy, link.setupHash, nextRefresh = true, setupHash, now + link.REFRESH_S
  local path = hotlap.referencePath(ids.car, ids.track, ids.layout, setupHash)
  local ok = pcall(web.get, link.url .. path, nil, answered(state, setupHash, ids.layout))
  if not ok then
    link.busy = false
    link.message = "No AI reference: web requests are not available."
  end
end

return link
