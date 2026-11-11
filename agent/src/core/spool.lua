-- Outgoing queue of spool lines. Lines wait here until the shell may write again; each write
-- is one new, immutable chunk file holding every line that was waiting. CSP can only write
-- whole files (there is no append), so a session is a directory of numbered chunks:
--   spool/<session-id>/000001.jsonl, 000002.jsonl, ...
-- Pure Lua: the shell does the actual io.saveAsync.

local spool = {}
spool.__index = spool

spool.MIN_INTERVAL = 0.2  -- seconds between writes: at most 5 per second

---@param minInterval number|nil seconds between writes
function spool.new(minInterval)
  return setmetatable({
    pending = {},
    count = 0,
    chunks = 0,
    lastWrite = nil,
    minInterval = minInterval or spool.MIN_INTERVAL,
  }, spool)
end

function spool:add(line)
  self.count = self.count + 1
  self.pending[self.count] = line
end

---Returns `fileName, text` when there is something to write and the interval has passed.
---@param now number a clock in seconds that keeps running while the game is paused
---@param force boolean|nil ignore the interval (shutdown)
function spool:take(now, force)
  if self.count == 0 then return nil end
  if not force and self.lastWrite and now - self.lastWrite < self.minInterval then return nil end
  local text = table.concat(self.pending, "\n", 1, self.count) .. "\n"
  for i = 1, self.count do self.pending[i] = nil end
  self.count = 0
  self.chunks = self.chunks + 1
  self.lastWrite = now
  return string.format("%06d.jsonl", self.chunks), text
end

return spool
