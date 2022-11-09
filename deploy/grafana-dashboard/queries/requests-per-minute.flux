import "date"

data = from(bucket: "poseidon/autogen")
  |> range(start: date.truncate(t: v.timeRangeStart, unit: 1m), stop: date.truncate(t: v.timeRangeStop, unit: 1m))
  |> filter(fn: (r) => r._field == "duration")
  |> filter(fn: (r) => (not exists r.environment_id) or contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))
  |> keep(columns: ["_time", "_value", "status"])

all = data |> set(key: "status", value: "all")

result = union(tables: [data, all])
  |> aggregateWindow(every: 1m, fn: count, createEmpty: true)

if int(v: v.windowPeriod) > int(v: 1m)
  then result |> aggregateWindow(every: duration(v: int(v: v.windowPeriod) * 2), fn: mean, createEmpty: true)
  else result |> aggregateWindow(every: duration(v: int(v: v.windowPeriod) * 5), fn: mean, createEmpty: false)
