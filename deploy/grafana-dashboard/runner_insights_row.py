from grafanalib.core import RowPanel, GridPos, Histogram, TimeSeries
from grafanalib.influxdb import InfluxDBTarget

from util import read_query

execution_duration_extra_json = {
    "fieldConfig": {
        "defaults": {
            "unit": "ns"
        }
    }
}
execution_duration = Histogram(
    title="Execution duration",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("execution-duration-hist"))],
    gridPos=GridPos(h=8, w=24, x=0, y=2),
    bucketSize=100000000,
    colorMode="palette-classic",
    extraJson=execution_duration_extra_json
)

executions_per_runner = Histogram(
    title="Executions per runner",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("executions-per-runner-hist"))],
    gridPos=GridPos(h=10, w=11, x=0, y=10),
    bucketSize=1,
    colorMode="palette-classic",
)

executions_per_minute = TimeSeries(
    title="Executions per minute",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("executions-per-minute-time"))],
    gridPos=GridPos(h=10, w=13, x=11, y=10),
)

request_body_size = TimeSeries(
    title="Request Body Size",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("request-body-size"))],
    gridPos=GridPos(h=10, w=11, x=0, y=20),
    scaleDistributionType="log",
    unit="bytes",
)

runner_per_minute = TimeSeries(
    title="Runner per minute",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("runner-per-minute"))],
    gridPos=GridPos(h=10, w=13, x=11, y=20),
)

runner_insights_row = RowPanel(
    title="Runner Insights",
    collapsed=True,
    gridPos=GridPos(h=1, w=24, x=0, y=1),
    panels=[
        execution_duration,
        executions_per_runner,
        executions_per_minute,
        request_body_size,
        runner_per_minute
    ]
)
