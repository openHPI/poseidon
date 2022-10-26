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

envMapping = from(bucket: "poseidon/autogen")
  |> range(start: -1y)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_environments")
  |> filter(fn: (r) => r["event_type"] == "creation")
  |> group(columns: ["id", "stage"], mode:"by")
  |> last()
  |> keep(columns: ["id", "image", "stage"])
  |> rename(columns: {id: "environment_id"})
  |> map(fn: (r) => ({ r with image: strings.trimPrefix(v: r.image, prefix: "openhpi/co_execenv_") + "(" + strings.substring(v: r.stage, start: 0, end: 1) + r.environment_id + ")" }))

join(tables: {key1: result, key2: envMapping}, on: ["environment_id", "stage"], method: "inner")
  |> keep(columns: ["_value", "image", "_time"])
  |> group(columns: ["image"], mode: "by")
  |> rename(columns: {_value: ""})
