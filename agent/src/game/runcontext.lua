-- What a session and a pass were driven under, read from CSP for the spool
-- (docs/adr/0028-run-context-and-ai-provenance.md): the agent's build, CSP's version, the cars in
-- the session, the AI's level, aggression and hints, and per pass the hint the AI was given,
-- damage and resets. Encoded by core/context_lines.lua.

local calibration = require("game.calibration")
local carlink = require("game.carlink")
local ghost = require("game.ghost")

local runcontext = {}

---The agent's build: the content hash tools/install_agent.py writes into src/build.lua when it
---installs the agent, or "dev" for a copy it did not install.
function runcontext.build()
  local ok, build = pcall(require, "build")
  if ok and type(build) == "table" and type(build.hash) == "string" and build.hash ~= "" then
    return build.hash
  end
  return "dev"
end

local function call(fn)
  if not fn then return nil end
  local ok, value = pcall(fn)
  return ok and value or nil
end

---The session line's context. In a human session the AI's settings are what it would drive
---with; only an AI session says which hint and grip it was given.
---@param source string "human" or "ai"
---@param car ac.StateCar
---@param sim ac.StateSim
function runcontext.session(source, car, sim)
  local ai = calibration.settings(ghost.pinnedShare())
  if source == "ai" then
    ai.extraGrip = ghost.EXTRA_AI_GRIP
  else
    ai.appliedHint, ai.pinnedShare = nil, nil
  end
  ai.level, ai.aggression = car.aiLevel, car.aiAggression
  local version, build = call(ac.getPatchVersion), call(ac.getPatchVersionCode)
  return {
    carsCount = sim.carsCount or 1, ai = ai,
    cspVersion = type(version) == "string" and version or nil,
    cspBuild = type(build) == "number" and build or nil,
  }
end

---A pass line's context, read when the pass ends.
---@param car ac.StateCar
function runcontext.pass(car)
  return {
    clock = carlink.clock, aiHint = ghost.active() and calibration.applied() or nil,
    damage = carlink.damage(car), resets = car.resetCounter or 0,
  }
end

return runcontext
