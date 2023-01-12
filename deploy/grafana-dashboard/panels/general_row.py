from grafanalib.core import RowPanel, GridPos, Stat, TimeSeries, Heatmap, BarGauge, GAUGE_DISPLAY_MODE_GRADIENT, \
    ORIENTATION_VERTICAL, GAUGE_DISPLAY_MODE_BASIC
from grafanalib.influxdb import InfluxDBTarget

from utils.color_mapping import grey_all_mapping, color_mapping_environments
from utils.utils import read_query

requests_per_minute = TimeSeries(
    title="Requests per minute",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("requests-per-minute"))],
    gridPos=GridPos(h=9, w=8, x=0, y=22),
    scaleDistributionType="log",
    extraJson=grey_all_mapping,
    lineInterpolation="smooth",
)

request_latency = Heatmap(
    title="Request Latency",
    dataSource="Flux",
    dataFormat="timeseries",
    targets=[InfluxDBTarget(query=read_query("request-latency"))],
    gridPos=GridPos(h=9, w=8, x=8, y=22),
    maxDataPoints=None,
    extraJson={
        "options": {},
        "yAxis": {
            "format": "ns"
        }
    },
)

service_time = TimeSeries(
    title="Service time (99.9%)",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("service-time"))],
    gridPos=GridPos(h=9, w=8, x=16, y=22),
    scaleDistributionType="log",
    scaleDistributionLog=10,
    unit="ns",
    maxDataPoints=None,
    lineInterpolation="smooth",
)

current_environment_count = Stat(
    title="Current environment count",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("current-environment-count"))],
    gridPos=GridPos(h=6, w=8, x=0, y=31),
    alignment="center",
)

currently_used_runners = Stat(
    title="Currently used runners",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("currently-used-runners"))],
    gridPos=GridPos(h=6, w=8, x=8, y=31),
    alignment="center",
)

number_of_executions = BarGauge(
    title="Number of Executions",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("number-of-executions", "environment-mapping"))],
    gridPos=GridPos(h=6, w=8, x=16, y=31),
    allValues=True,
    orientation=ORIENTATION_VERTICAL,
    displayMode=GAUGE_DISPLAY_MODE_BASIC,
    max=None,
    extraJson=color_mapping_environments,
)

execution_duration = BarGauge(
    title="Execution duration",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("execution-duration", "environment-mapping"))],
    gridPos=GridPos(h=11, w=8, x=0, y=37),
    allValues=True,
    displayMode=GAUGE_DISPLAY_MODE_GRADIENT,
    format="ns",
    max=None,
    decimals=2,
    extraJson=color_mapping_environments,
)

executions_per_runner = BarGauge(
    title="Executions per runner",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("executions-per-runner", "environment-mapping"))],
    gridPos=GridPos(h=11, w=8, x=8, y=37),
    allValues=True,
    displayMode=GAUGE_DISPLAY_MODE_GRADIENT,
    max=None,
    decimals=2,
    extraJson=color_mapping_environments,
)

executions_per_minute = BarGauge(
    title="Executions per minute",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("executions-per-minute", "environment-mapping"))],
    gridPos=GridPos(h=11, w=8, x=16, y=37),
    allValues=True,
    displayMode=GAUGE_DISPLAY_MODE_GRADIENT,
    max=None,
    decimals=2,
    extraJson=color_mapping_environments,
)

general_row = RowPanel(
    title="General",
    collapsed=False,
    gridPos=GridPos(h=1, w=24, x=0, y=21),
)

general_panels = [
    general_row,
    requests_per_minute,
    request_latency,
    service_time,
    current_environment_count,
    currently_used_runners,
    number_of_executions,
    execution_duration,
    executions_per_runner,
    executions_per_minute,
]
