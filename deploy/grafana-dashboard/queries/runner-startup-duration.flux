import "strings"

result = from(bucket: "poseidon")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_nomad_idle_runners")
  |> filter(fn: (r) => r["_field"] == "startup_duration")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> keep(columns: ["_value", "_time", "environment_id", "stage"])
  |> aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)
