-- The raw channels sampled while the car is in a corner zone, in spool column order.
-- The names are the column names of contracts/spool_pass.schema.json; a test keeps them equal.
-- Pure Lua.

local channels = {}

-- { column name, decimals kept in the spool }
channels.list = {
  { "t", 4 },       -- seconds since the pass started (physics time when CSP gives it)
  { "d", 2 },       -- metres from the start line
  { "v", 3 },       -- m/s
  { "gas", 3 },
  { "brake", 3 },
  { "steer", 2 },   -- steering wheel angle, degrees
  { "lat_g", 3 },
  { "lon_g", 3 },
  { "yaw", 4 },     -- rad/s
  { "sa_fl", 3 }, { "sa_fr", 3 }, { "sa_rl", 3 }, { "sa_rr", 3 },          -- slip angle, degrees
  { "sr_fl", 4 }, { "sr_fr", 4 }, { "sr_rl", 4 }, { "sr_rr", 4 },          -- slip ratio
  { "load_fl", 0 }, { "load_fr", 0 }, { "load_rl", 0 }, { "load_rr", 0 },  -- tyre load, N
  { "out", 0 },     -- wheels outside the track
  { "rh_f", 4 },    -- ride height, m
  { "rh_r", 4 },
  -- added with docs/adr/0028-run-context-and-ai-provenance.md
  { "nd_fl", 3 }, { "nd_fr", 3 }, { "nd_rl", 3 }, { "nd_rr", 3 },  -- normalised slip, 1 = peak grip
  { "w_fl", 2 }, { "w_fr", 2 }, { "w_rl", 2 }, { "w_rr", 2 },      -- wheel angular speed, rad/s
  { "st_fl", 4 }, { "st_fr", 4 }, { "st_rl", 4 }, { "st_rr", 4 },  -- suspension travel
  { "vx", 3 }, { "vz", 3 },  -- velocity in the car's frame, m/s (sideways, forwards)
  { "x", 2 }, { "z", 2 },    -- world position, m
  { "gear", 0 },
  { "rpm", 0 },
}

channels.names = {}
for i, channel in ipairs(channels.list) do channels.names[i] = channel[1] end

-- Sampled with the pass but not written per frame: the pass line carries only a summary (the
-- "ers" and "context" objects of the contract).
channels.summarised = {
  "kers_charge",  -- battery charge, 0..1
  "kers_input",   -- deployment, 0..1
  "abs",          -- 1 while ABS acts
  "tc",           -- 1 while traction control acts
  "drs",          -- 1 while DRS is open
}

-- every field the recorder keeps per frame
channels.recorded = {}
for i, name in ipairs(channels.names) do channels.recorded[i] = name end
for _, name in ipairs(channels.summarised) do channels.recorded[#channels.recorded + 1] = name end

return channels
