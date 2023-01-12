import "strings"

myWindowPeriod = if int(v: v.windowPeriod) > int(v: 1m) then duration(v: int(v: v.windowPeriod) * 10) else duration(v: int(v: v.windowPeriod) * 5)
data = from(bucket: "poseidon")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_file_download")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))

actual = data |> filter(fn: (r) => r["_field"] == "actual_length")
expected = data |> filter(fn: (r) => r["_field"] == "expected_length")
result = join(tables: {key1: actual, key2: expected}, on: ["_time", "environment_id", "runner_id", "stage"], method: "inner")
  |> map(fn: (r) => ({ _value: if r._value_key2 == 0 then 1.0 else float(v: r._value_key1) / float(v: r._value_key2), environment_id: r.environment_id, runner_id: r.runner_id, stage: r.stage }))
