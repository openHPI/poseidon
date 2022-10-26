from grafanalib.core import RowPanel, GridPos, Stat, TimeSeries, Heatmap, BarGauge, GAUGE_DISPLAY_MODE_GRADIENT
from grafanalib.influxdb import InfluxDBTarget

from color_mapping import grey_all_mapping
from util import read_query

requests_per_minute = TimeSeries(
    title="Requests per minute",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("requests-per-minute"))],
    gridPos=GridPos(h=9, w=8, x=0, y=1),
    scaleDistributionType="log",
    extraJson=grey_all_mapping
)

request_latency = Heatmap(
    title="Request Latency",
    dataSource='Poseidon',
    dataFormat="timeseries",
    targets=[InfluxDBTarget(query=read_query("request-latency"))],
    gridPos=GridPos(h=9, w=8, x=8, y=1),
    extraJson={
        "options": {},
        "yAxis": {
            "format": "ns"
        }
    }
)

service_time = TimeSeries(
    title="Service time (99.9%)",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("service-time"))],
    gridPos=GridPos(h=9, w=8, x=16, y=1),
    scaleDistributionType="log",
    scaleDistributionLog=10,
    unit="ns"
)

current_environment_count = Stat(
    title="Current environment count",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("current-environment-count"))],
    gridPos=GridPos(h=6, w=8, x=0, y=10),
    alignment='center'
)

currently_used_runners = Stat(
    title="Currently used runners",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("currently-used-runners"))],
    gridPos=GridPos(h=6, w=8, x=8, y=10),
    alignment="center"
)

number_of_executions = BarGauge(
    title="Number of Executions",
    dataSource="Poseidon",
    targets=[InfluxDBTarget(query=read_query("number-of-executions"))],
    gridPos=GridPos(h=6, w=8, x=16, y=10),
    allValues=True,
    displayMode=GAUGE_DISPLAY_MODE_GRADIENT,
    max=None,
)

execution_duration = BarGauge(
    title="Execution duration",
    dataSource="Poseidon",
    targets=[InfluxDBTarget(query=read_query("execution-duration"))],
    gridPos=GridPos(h=11, w=8, x=0, y=16),
    allValues=True,
    displayMode=GAUGE_DISPLAY_MODE_GRADIENT,
    format="ns",
    max=None,
)

executions_per_runner = BarGauge(
    title="Executions per runner",
    dataSource="Poseidon",
    targets=[InfluxDBTarget(query=read_query("executions-per-runner"))],
    gridPos=GridPos(h=11, w=8, x=8, y=16),
    allValues=True,
    displayMode=GAUGE_DISPLAY_MODE_GRADIENT,
    max=None,
)

executions_per_minute = BarGauge(
    title="Executions per minute",
    dataSource="Poseidon",
    targets=[InfluxDBTarget(query=read_query("executions-per-minute"))],
    gridPos=GridPos(h=11, w=8, x=16, y=16),
    allValues=True,
    displayMode=GAUGE_DISPLAY_MODE_GRADIENT,
    max=None,
)

general_row = RowPanel(
    title="General",
    collapsed=True,
    gridPos=GridPos(h=1, w=24, x=0, y=0),
    panels=[
        requests_per_minute,
        request_latency,
        service_time,
        current_environment_count,
        currently_used_runners,
        number_of_executions,
        execution_duration,
        executions_per_runner,
        executions_per_minute
    ]
)
