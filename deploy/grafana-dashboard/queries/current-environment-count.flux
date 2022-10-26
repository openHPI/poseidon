import "date"

// The need for the date truncation is caused by Poseidon sending all influx events at the same time when starting up. This way not the last but a random value is displayed.
// Since in this startup process the highest value is the correct one, we choose the highest value of the last events.

data = from(bucket: "poseidon/autogen")
  |> range(start: -1y)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_environments")
  |> group(columns: ["stage"], mode:"by")
  |> map(fn: (r) => ({ r with _time: date.truncate(t: r._time, unit: 1m) }))

deploy_times = data
  |> last()
  |> keep(columns: ["stage", "_time"])

join(tables: {key1: data, key2: deploy_times}, on: ["stage", "_time"], method: "inner")
  |> max()
  |> keep(columns: ["stage", "_value"])
  |> rename(columns: {_value: ""})
