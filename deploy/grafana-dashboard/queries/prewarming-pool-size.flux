import "strings"

result = from(bucket: "poseidon")
  |> range(start: -1y)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_poolsize")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> group(columns: ["environment_id", "stage"], mode:"by")
  |> last()
  |> keep(columns: ["_value", "environment_id", "stage"])
