import "strings"
import "date"

myWindowPeriod = if int(v: v.windowPeriod) > int(v: 2m) then duration(v: int(v: v.windowPeriod) * 30) else duration(v: int(v: v.windowPeriod) * 15)
result = from(bucket: "poseidon/autogen")
  |> range(start: date.truncate(t: v.timeRangeStart, unit: 1m), stop: date.truncate(t: v.timeRangeStop, unit: 1m))
  |> filter(fn: (r) => r["_measurement"] == "poseidon_used_runners")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))
  |> filter(fn: (r) => r["event_type"] == "creation")
  |> group(columns: ["environment_id", "stage"], mode:"by")
  |> aggregateWindow(every: 1m, fn: count, createEmpty: true)
  |> keep(columns: ["_value", "_time", "environment_id", "stage"])
  |> aggregateWindow(every: myWindowPeriod, fn: mean, createEmpty: true)
