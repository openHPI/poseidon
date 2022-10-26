import "strings"

myWindowPeriod = if int(v: v.windowPeriod) >= int(v: 30s) then duration(v: int(v: v.windowPeriod) * 5) else v.windowPeriod
result = from(bucket: "poseidon/autogen")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_nomad_idle_runners" and r["_field"] == "count")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))
  |> keep(columns: ["_value", "_time", "environment_id", "stage"])
  |> aggregateWindow(every: myWindowPeriod, fn: min, createEmpty: false)

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
