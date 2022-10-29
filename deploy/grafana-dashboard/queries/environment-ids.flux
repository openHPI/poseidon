from(bucket: "poseidon/autogen")
  |> range(start: -1y)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_environments")
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))
  |> keep(columns: ["id"])
  |> distinct(column: "id")
  |> keep(columns: ["_value"])
