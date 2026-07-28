package distributed

import "github.com/redis/go-redis/v9"

var (
	createScript      = redis.NewScript(createLua)
	acknowledgeScript = redis.NewScript(acknowledgeLua)
	beginScript       = redis.NewScript(beginLua)
	commitScript      = redis.NewScript(commitLua)
	closeScript       = redis.NewScript(closeLua)
	settleScript      = redis.NewScript(settleLua)
	snapshotScript    = redis.NewScript(snapshotLua)
)

const createLua = `
for i = 1, #KEYS do
  if redis.call('EXISTS', KEYS[i]) ~= 0 then return {1} end
end
redis.call('HSET', KEYS[1],
  'version', ARGV[1], 'state', 'open', 'algorithm', ARGV[2],
  'seed', ARGV[3], 'total', ARGV[4], 'created_ms', ARGV[5],
  'deadline_ms', ARGV[6], 'attempts', 0, 'retries', 0,
  'duplicates', 0, 'work_nanos', 0, 'updated_ms', ARGV[5],
  'policy', ARGV[7], 'delivery_limit', ARGV[8], 'delivery_reserved', 0)
return {0}`

const acknowledgeLua = `
if redis.call('EXISTS', KEYS[1]) == 0 then return {1} end
if redis.call('HGET', KEYS[1], 'state') ~= 'open' then return {2} end
local first, last = tonumber(ARGV[1]), tonumber(ARGV[2])
local total = tonumber(redis.call('HGET', KEYS[1], 'total'))
if not first or not last or first < 1 or last < first or last > total then return {3} end
local added = 0
for ordinal = first, last do
  local member = tostring(ordinal)
  local seen = false
  for i = 2, 6 do
    if redis.call('SISMEMBER', KEYS[i], member) == 1 then seen = true break end
  end
  if not seen then redis.call('SADD', KEYS[2], member) added = added + 1 end
end
if added > 0 then redis.call('HSET', KEYS[1], 'updated_ms', ARGV[3]) end
return {0, added}`

const beginLua = `
if redis.call('EXISTS', KEYS[1]) == 0 then return {9} end
local ordinal, kind, now = ARGV[1], ARGV[2], tonumber(ARGV[3])
local total = tonumber(redis.call('HGET', KEYS[1], 'total'))
local number = tonumber(ordinal)
if not number or number < 1 or number > total or (kind ~= 'initial' and kind ~= 'retry') then return {10} end
local function counters(code)
  return {code, tonumber(redis.call('HGET', KEYS[1], 'attempts')), tonumber(redis.call('HGET', KEYS[1], 'retries')), tonumber(redis.call('HGET', KEYS[1], 'duplicates'))}
end
if redis.call('SISMEMBER', KEYS[4], ordinal) == 1 then redis.call('HINCRBY', KEYS[1], 'duplicates', 1) return counters(2) end
if redis.call('SISMEMBER', KEYS[5], ordinal) == 1 then redis.call('HINCRBY', KEYS[1], 'duplicates', 1) return counters(3) end
if redis.call('SISMEMBER', KEYS[6], ordinal) == 1 then redis.call('HINCRBY', KEYS[1], 'duplicates', 1) return counters(4) end
local state = redis.call('HGET', KEYS[1], 'state')
local policy = redis.call('HGET', KEYS[1], 'policy')
if policy == 'one_shot_delivery' and redis.call('SISMEMBER', KEYS[3], ordinal) == 1 then
  redis.call('HINCRBY', KEYS[1], 'duplicates', 1)
  return counters(7)
end
if kind == 'retry' and redis.call('SISMEMBER', KEYS[3], ordinal) == 1 then
  if state == 'closed' and now >= tonumber(redis.call('HGET', KEYS[1], 'deadline_ms')) then return counters(6) end
  redis.call('HINCRBY', KEYS[1], 'attempts', 1)
  redis.call('HINCRBY', KEYS[1], 'retries', 1)
  redis.call('HSET', KEYS[1], 'updated_ms', ARGV[3])
  return counters(1)
end
if kind == 'initial' and redis.call('SISMEMBER', KEYS[2], ordinal) == 1 then
  if state ~= 'open' then
    redis.call('SMOVE', KEYS[2], KEYS[6], ordinal)
    redis.call('HINCRBY', KEYS[1], 'duplicates', 1)
    return counters(4)
  end
  if policy == 'one_shot_delivery' then
    local reserved = tonumber(redis.call('HGET', KEYS[1], 'delivery_reserved'))
    local delivery_limit = tonumber(redis.call('HGET', KEYS[1], 'delivery_limit'))
    if reserved >= delivery_limit then return counters(8) end
    redis.call('HINCRBY', KEYS[1], 'delivery_reserved', 1)
  end
  redis.call('SMOVE', KEYS[2], KEYS[3], ordinal)
  redis.call('HINCRBY', KEYS[1], 'attempts', 1)
  redis.call('HSET', KEYS[1], 'updated_ms', ARGV[3])
  return counters(0)
end
if redis.call('SISMEMBER', KEYS[3], ordinal) == 1 then redis.call('HINCRBY', KEYS[1], 'duplicates', 1) return counters(5) end
return counters(5)`

const commitLua = `
if redis.call('EXISTS', KEYS[1]) == 0 then return {3} end
local ordinal, terminal = ARGV[1], ARGV[2]
local total = tonumber(redis.call('HGET', KEYS[1], 'total'))
local number = tonumber(ordinal)
if not number or number < 1 or number > total then return {4} end
if terminal ~= 'completed' and terminal ~= 'failed' then return {4} end
if redis.call('SISMEMBER', KEYS[4], ordinal) == 1 or redis.call('SISMEMBER', KEYS[5], ordinal) == 1 or redis.call('SISMEMBER', KEYS[6], ordinal) == 1 then return {1} end
if redis.call('SISMEMBER', KEYS[3], ordinal) == 0 then return {2} end
redis.call('SREM', KEYS[3], ordinal)
if terminal == 'completed' then
  redis.call('SADD', KEYS[4], ordinal)
  redis.call('HSET', KEYS[7], ordinal, ARGV[4])
  redis.call('HINCRBY', KEYS[1], 'work_nanos', ARGV[5])
elseif terminal == 'failed' then
  redis.call('SADD', KEYS[5], ordinal)
  redis.call('HSET', KEYS[8], ordinal, ARGV[3])
end
if redis.call('HGET', KEYS[1], 'state') == 'open' and
   redis.call('SCARD', KEYS[2]) == 0 and redis.call('SCARD', KEYS[3]) == 0 and
   redis.call('SCARD', KEYS[4]) + redis.call('SCARD', KEYS[5]) + redis.call('SCARD', KEYS[6]) == total then
  redis.call('HSET', KEYS[1], 'state', 'closed')
end
redis.call('HSET', KEYS[1], 'updated_ms', ARGV[6])
return {0}`

const closeLua = `
if redis.call('EXISTS', KEYS[1]) == 0 then return {2} end
if redis.call('HGET', KEYS[1], 'state') == 'closed' then return {1, 0} end
redis.call('HSET', KEYS[1], 'state', 'closed', 'closed_ms', ARGV[1], 'close_reason', ARGV[2], 'updated_ms', ARGV[1])
local pending = redis.call('SMEMBERS', KEYS[2])
for _, ordinal in ipairs(pending) do redis.call('SMOVE', KEYS[2], KEYS[6], ordinal) end
return {0, #pending}`

const settleLua = `
if redis.call('EXISTS', KEYS[1]) == 0 then return {4} end
if redis.call('HGET', KEYS[1], 'state') ~= 'closed' then return {3} end
if tonumber(ARGV[1]) < tonumber(redis.call('HGET', KEYS[1], 'deadline_ms')) then return {1} end
local active = redis.call('SMEMBERS', KEYS[3])
if #active == 0 then return {2, 0} end
local reason = ARGV[2]
if redis.call('HGET', KEYS[1], 'policy') == 'one_shot_delivery' then reason = 'delivery_indeterminate' end
for _, ordinal in ipairs(active) do
  redis.call('SMOVE', KEYS[3], KEYS[5], ordinal)
  redis.call('HSET', KEYS[8], ordinal, reason)
end
redis.call('HSET', KEYS[1], 'updated_ms', ARGV[1])
return {0, #active}`

const snapshotLua = `
if redis.call('EXISTS', KEYS[1]) == 0 then return {1} end
local counts = {}
local sum = 0
for i = 2, 6 do counts[i] = redis.call('SCARD', KEYS[i]) sum = sum + counts[i] end
local total = tonumber(redis.call('HGET', KEYS[1], 'total'))
if sum > total then return {2} end
for i = 2, 5 do
  for j = i + 1, 6 do
    if redis.call('SINTERCARD', 2, KEYS[i], KEYS[j], 'LIMIT', 1) ~= 0 then return {2} end
  end
end
local rejected, indeterminate, transport = 0, 0, 0
for _, reason in ipairs(redis.call('HVALS', KEYS[8])) do
  if reason == 'delivery_rejected' then rejected = rejected + 1 end
  if reason == 'delivery_indeterminate' then indeterminate = indeterminate + 1 end
  if reason == 'delivery_transport' then transport = transport + 1 end
end
local meta = redis.call('HMGET', KEYS[1], 'version', 'state', 'algorithm', 'seed', 'total', 'created_ms', 'deadline_ms', 'attempts', 'retries', 'duplicates', 'work_nanos', 'updated_ms', 'policy', 'delivery_limit', 'delivery_reserved')
if tostring(meta[1]) ~= tostring(ARGV[1]) then return {2} end
return {0, meta[2], meta[3], meta[4], meta[5], meta[6], meta[7], counts[2], counts[3], counts[4], counts[5], counts[6], meta[8], meta[9], meta[10], meta[11], meta[12], meta[13], meta[14], meta[15], rejected, indeterminate, transport}`
