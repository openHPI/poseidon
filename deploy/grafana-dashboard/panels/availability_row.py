from grafanalib.core import RowPanel, BarGauge, GridPos, TimeSeries, ORIENTATION_VERTICAL, \
    GAUGE_DISPLAY_MODE_BASIC
from grafanalib.influxdb import InfluxDBTarget

from utils.color_mapping import color_mapping_environments
from utils.utils import read_query

prewarming_pool_size = BarGauge(
    title="Prewarming Pool Size",
    dataSource="Poseidon",
    targets=[InfluxDBTarget(query=read_query("prewarming-pool-size", "environment-mapping"))],
    gridPos=GridPos(h=10, w=11, x=0, y=1),
    allValues=True,
    orientation=ORIENTATION_VERTICAL,
    displayMode=GAUGE_DISPLAY_MODE_BASIC,
    max=None,
    extraJson=color_mapping_environments,
)

idle_runner = TimeSeries(
    title="Idle Runner",
    dataSource="Poseidon",
    targets=[InfluxDBTarget(query=read_query("idle-runner", "environment-mapping"))],
    gridPos=GridPos(h=10, w=13, x=11, y=1),
    lineInterpolation="stepAfter",
    maxDataPoints=None,
    extraJson=color_mapping_environments,
)

runner_startup_duration = TimeSeries(
    title="Runner startup duration",
    dataSource="Poseidon",
    targets=[InfluxDBTarget(query=read_query("runner-startup-duration", "environment-mapping"))],
    gridPos=GridPos(h=10, w=12, x=0, y=11),
    unit="ns",
    maxDataPoints=None,
    lineInterpolation="smooth",
    extraJson=color_mapping_environments,
)

used_runner = TimeSeries(
    title="Used Runner",
    dataSource="Poseidon",
    targets=[InfluxDBTarget(query=read_query("used-runner"))],
    gridPos=GridPos(h=10, w=12, x=12, y=11),
    maxDataPoints=None,
    lineInterpolation="smooth",
)

availability_row = RowPanel(
    title="Availability",
    collapsed=False,
    gridPos=GridPos(h=1, w=24, x=0, y=0),
)

availability_panels = [
    availability_row,
    prewarming_pool_size,
    idle_runner,
    runner_startup_duration,
    used_runner,
]
