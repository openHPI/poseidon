import "strings"

data = from(bucket: "poseidon")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))

runner_deletions = data
  |> filter(fn: (r) => r["_measurement"] == "poseidon_used_runners")
  |> filter(fn: (r) => r["event_type"] == "deletion")
  |> keep(columns: ["id", "stage"])
  |> rename(columns: {id: "runner_id"})

executions = data
  |> filter(fn: (r) => r["_measurement"] == "poseidon_nomad_executions" or r["_measurement"] == "poseidon_aws_executions")
  |> filter(fn: (r) => r["event_type"] == "creation")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> keep(columns: ["_value", "environment_id", "runner_id"])
  |> count()

result = join(tables: {key1: executions, key2: runner_deletions}, on: ["runner_id"], method: "inner")
  |> keep(columns: ["_value", "environment_id", "stage"])
  |> mean()
