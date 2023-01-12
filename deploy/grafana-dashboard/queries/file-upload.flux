import "strings"

myWindowPeriod = if int(v: v.windowPeriod) > int(v: 1m) then duration(v: int(v: v.windowPeriod) * 10) else duration(v: int(v: v.windowPeriod) * 5)
result = from(bucket: "poseidon")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_field"] == "request_size")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))
  |> keep(columns: ["_time", "_value", "environment_id", "stage"])
  |> aggregateWindow(every: myWindowPeriod, fn: mean, createEmpty: false)
