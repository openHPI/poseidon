import "strings"

data = from(bucket: "poseidon/autogen")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => (not exists r.stage) or contains(value: r["stage"], set: ${stages:json}))

runner_deletions = data
  |> filter(fn: (r) => r["_measurement"] == "poseidon_used_runners")
  |> filter(fn: (r) => r["event_type"] == "deletion")
  |> keep(columns: ["_time", "id", "stage"])
  |> rename(columns: {id: "runner_id"})

executions = data
  |> filter(fn: (r) => r["_measurement"] == "poseidon_nomad_executions" or r["_measurement"] == "poseidon_aws_executions")
  |> filter(fn: (r) => r["event_type"] == "creation")
  |> filter(fn: (r) => contains(value: r["environment_id"], set: ${environment_ids:json}))
  |> keep(columns: ["_value", "environment_id", "runner_id"])
  |> count()

result = join(tables: {key1: executions, key2: runner_deletions}, on: ["runner_id"], method: "inner")
  |> keep(columns: ["_value", "_time", "environment_id", "stage"])
  |> aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)

envMapping = from(bucket: "poseidon/autogen")
  |> range(start: -1y)
  |> filter(fn: (r) => r["_measurement"] == "poseidon_environments")
  |> filter(fn: (r) => r["event_type"] == "creation")
  |> group(columns: ["id", "stage"], mode:"by")
  |> last()
  |> keep(columns: ["id", "image", "stage"])
  |> rename(columns: {id: "environment_id"})
  |> map(fn: (r) => ({ r with image: strings.substring(v: r.stage, start: 0, end: 1) + r.environment_id + "/" + strings.trimPrefix(v: r.image, prefix: "openhpi/co_execenv_")}))

join(tables: {key1: result, key2: envMapping}, on: ["environment_id", "stage"], method: "inner")
  |> keep(columns: ["_value", "image", "_time"])
  |> group(columns: ["image"], mode: "by")
  |> rename(columns: {_value: ""})
