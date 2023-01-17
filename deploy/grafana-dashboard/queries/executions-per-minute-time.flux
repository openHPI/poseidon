import "strings"
import "date"

result = from(bucket: "poseidon")
  |> range(start: date.truncate(t: v.timeRangeStart, unit: 1m), stop: date.truncate(t: v.timeRangeStop, unit: 1m))
  |> filter(fn: (r) => r["_measurement"] == "poseidon_aws_executions" or r["_measurement"] == "poseidon_nomad_executions")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => r["event_type"] == "creation")
  |> group(columns: ["environment_id", "stage"], mode:"by")
  |> aggregateWindow(every: 1m, fn: count, createEmpty: true)
  |> aggregateWindow(every: duration(v: int(v: v.windowPeriod) * 8), fn: mean, createEmpty: true)
