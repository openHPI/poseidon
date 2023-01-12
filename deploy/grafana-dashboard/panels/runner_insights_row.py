from grafanalib.core import RowPanel, GridPos, Histogram, TimeSeries, BarGauge, ORIENTATION_VERTICAL, \
    GAUGE_DISPLAY_MODE_BASIC, PERCENT_UNIT_FORMAT, GAUGE_CALC_MEAN
from grafanalib.influxdb import InfluxDBTarget

from utils.color_mapping import color_mapping_environments
from utils.utils import read_query, deep_update_dict

execution_duration_extra_json = {
    "fieldConfig": {
        "defaults": {
            "unit": "ns"
        }
    }
}
deep_update_dict(execution_duration_extra_json, color_mapping_environments)
execution_duration = Histogram(
    title="Execution duration",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("execution-duration-hist", "environment-mapping"))],
    gridPos=GridPos(h=8, w=24, x=0, y=49),
    bucketSize=100000000,
    colorMode="palette-classic",
    fillOpacity=50,
    lineWidth=1,
    maxDataPoints=None,
    extraJson=execution_duration_extra_json,
)

executions_per_runner = Histogram(
    title="Executions per runner",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("executions-per-runner-hist", "environment-mapping"))],
    gridPos=GridPos(h=10, w=11, x=0, y=57),
    bucketSize=1,
    colorMode="palette-classic",
    fillOpacity=50,
    lineWidth=1,
    maxDataPoints=None,
    extraJson=color_mapping_environments,
)

executions_per_minute = TimeSeries(
    title="Executions per minute",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("executions-per-minute-time", "environment-mapping"))],
    gridPos=GridPos(h=10, w=13, x=11, y=57),
    maxDataPoints=None,
    lineInterpolation="smooth",
    extraJson=color_mapping_environments,
)

file_upload = TimeSeries(
    title="File Upload",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("file-upload", "environment-mapping"))],
    gridPos=GridPos(h=10, w=11, x=0, y=67),
    scaleDistributionType="log",
    unit="bytes",
    maxDataPoints=None,
    lineInterpolation="smooth",
    extraJson=color_mapping_environments,
)

runner_per_minute = TimeSeries(
    title="Runner per minute",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("runner-per-minute", "environment-mapping"))],
    gridPos=GridPos(h=10, w=13, x=11, y=67),
    maxDataPoints=None,
    lineInterpolation="smooth",
    extraJson=color_mapping_environments,
)

file_download = TimeSeries(
    title="File Download",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("file-download", "environment-mapping"))],
    gridPos=GridPos(h=10, w=11, x=0, y=77),
    scaleDistributionType="log",
    unit="bytes",
    maxDataPoints=None,
    lineInterpolation="smooth",
    extraJson=color_mapping_environments,
)

file_download_ratio = BarGauge(
    title="File Download Ratio",
    dataSource="Flux",
    targets=[InfluxDBTarget(query=read_query("file-download-ratio", "environment-mapping"))],
    gridPos=GridPos(h=10, w=13, x=11, y=77),
    max=1,
    allValues=False,
    calc=GAUGE_CALC_MEAN,
    orientation=ORIENTATION_VERTICAL,
    displayMode=GAUGE_DISPLAY_MODE_BASIC,
    format=PERCENT_UNIT_FORMAT,
    extraJson=color_mapping_environments,
)

runner_insights_row = RowPanel(
    title="Runner Insights",
    collapsed=False,
    gridPos=GridPos(h=1, w=24, x=0, y=48),
)

runner_insights_panels = [
    runner_insights_row,
    execution_duration,
    executions_per_runner,
    executions_per_minute,
    file_upload,
    runner_per_minute,
    file_download,
    file_download_ratio,
]
