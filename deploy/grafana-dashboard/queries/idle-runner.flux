import "strings"

myWindowPeriod = if int(v: v.windowPeriod) >= int(v: 30s) then duration(v: int(v: v.windowPeriod) * 5) else v.windowPeriod
result = from(bucket: "poseidon/autogen")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_nomad_idle_runners" and r["_field"] == "count")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))
  |> keep(columns: ["_value", "_time", "environment_id", "stage"])
  |> aggregateWindow(every: myWindowPeriod, fn: min, createEmpty: false)
