import "strings"

result = from(bucket: "poseidon")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_field"] == "duration")
  |> filter(fn: (r) => r["_measurement"] == "poseidon_/execute" or r["_measurement"] == "poseidon_/files" or r["_measurement"] == "poseidon_/websocket")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => exists r.environment_id)
  |> keep(columns: ["_value", "runner_id", "environment_id", "stage"])
  |> group(columns: ["environment_id", "stage"])
  |> mean()
  |> map(fn: (r) => ({r with _value: r._value * 3.0})) // Each execution has three requests
