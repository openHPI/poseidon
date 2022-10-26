from(bucket: "poseidon/autogen")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_field"] == "duration")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))
  |> keep(columns: ["_time", "_value", "_measurement"])
  |> aggregateWindow(every: duration(v: int(v: v.windowPeriod) * 10), fn: (tables=<-, column) => tables |> quantile(q: 0.999))
