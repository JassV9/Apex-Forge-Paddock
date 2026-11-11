-- The raw channels sampled while the car is in a corner zone, in spool column order.
-- The names are the column names of contracts/spool_pass.schema.json; a test keeps them equal.
-- Pure Lua.

local channels = {}

-- { column name, decimals kept in the spool }
channels.list = {
  { "t", 4 },       -- seconds since the pass started
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
}

channels.names = {}
for i, channel in ipairs(channels.list) do channels.names[i] = channel[1] end

return channels
