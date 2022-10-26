import "strings"

result = from(bucket: "poseidon/autogen")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_field"] == "duration")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))
  |> filter(fn: (r) => r["_measurement"] == "poseidon_/execute" or r["_measurement"] == "poseidon_/files" or r["_measurement"] == "poseidon_/websocket")
  |> filter(fn: (r) => exists r.environment_id)
  |> keep(columns: ["_time", "_value", "environment_id", "stage"])
  |> aggregateWindow(every: v.windowPeriod, fn: mean)
  |> map(fn: (r) => ({r with _value: r._value * 3.0})) // Each execution has three requests

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
