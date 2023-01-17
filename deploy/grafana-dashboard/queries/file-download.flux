import "strings"

myWindowPeriod = if int(v: v.windowPeriod) > int(v: 1m) then duration(v: int(v: v.windowPeriod) * 100) else duration(v: int(v: v.windowPeriod) * 5)
result = from(bucket: "poseidon")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_file_download")
  |> filter(fn: (r) => r["_field"] == "actual_length")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> keep(columns: ["_time", "_value", "environment_id", "stage"])
