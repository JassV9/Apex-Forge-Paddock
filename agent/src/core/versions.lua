-- Setup versions: the fingerprint of what's on the car, and the next file name in a line
-- (base -> base_v2 -> base_v3, COTA_Quali_C4_V2 -> COTA_Quali_C4_V3). Pure Lua.

local versions = {}

---Fingerprint of the handling items' values (tab-filtered, so ERS maps or dash options
---changing don't make a "new setup").
---@param items table[] catalog items to include
---@param current table name -> value
function versions.key(items, current)
  local names = {}
  for _, it in ipairs(items) do names[#names + 1] = it.name end
  table.sort(names)
  local h = 5381
  for _, n in ipairs(names) do
    local s = n .. "=" .. tostring(current[n]) .. ";"
    for i = 1, #s do h = (h * 33 + s:byte(i)) % 2147483647 end
  end
  return string.format("s%08x", h)   -- never all digits (JSON key safety)
end

---Strip folders and extension.
function versions.stem(path)
  if not path or path == "" then return nil end
  local name = path:match("([^\\/]+)$") or path
  return (name:gsub("%.[iI][nN][iI]$", ""))
end

---Next name in the line, keeping the user's own style (_v2 / _V2 / -v2).
function versions.nextName(stem)
  if not stem or stem == "" then return "ApexForge_v2" end
  local head, sep, v, num = stem:match("^(.-)([_%- ]?)([vV])(%d+)$")
  if head and head ~= "" then
    return head .. sep .. v .. tostring(tonumber(num) + 1)
  end
  return stem .. "_v2"
end

---A name that doesn't exist yet: bumps the number while `exists(name)` says it's taken.
function versions.freeName(stem, exists)
  local name = versions.nextName(stem)
  for _ = 1, 50 do
    if not exists(name) then return name end
    name = versions.nextName(name)
  end
  return name
end

---Safe file-name text from a title the LLM wrote.
function versions.clean(text, maxLen)
  local s = tostring(text or ""):gsub("[^%w%-_ ]", ""):gsub("%s+", "_")
  return s:sub(1, maxLen or 40)
end

return versions
