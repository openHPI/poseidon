from grafanalib.core import RowPanel, BarGauge, GridPos, TimeSeries, ORIENTATION_VERTICAL, \
    GAUGE_DISPLAY_MODE_BASIC
from grafanalib.influxdb import InfluxDBTarget

from utils.utils import read_query

prewarming_pool_size = BarGauge(
    title="Prewarming Pool Size",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("prewarming-pool-size"))],
    gridPos=GridPos(h=10, w=11, x=0, y=3),
    allValues=True,
    orientation=ORIENTATION_VERTICAL,
    displayMode=GAUGE_DISPLAY_MODE_BASIC,
    max=None,
)

idle_runner = TimeSeries(
    title="Idle Runner",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("idle-runner"))],
    gridPos=GridPos(h=10, w=13, x=11, y=3),
    lineInterpolation="stepAfter",
    maxDataPoints=None
)

runner_startup_duration = TimeSeries(
    title="Runner startup duration",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("runner-startup-duration"))],
    gridPos=GridPos(h=10, w=12, x=0, y=13),
    unit="ns",
)

used_runner = TimeSeries(
    title="Used Runner",
    dataSource='Poseidon',
    targets=[InfluxDBTarget(query=read_query("used-runner"))],
    gridPos=GridPos(h=10, w=12, x=12, y=13),
    maxDataPoints=None
)

availability_row = RowPanel(
    title="Availability",
    collapsed=True,
    gridPos=GridPos(h=1, w=24, x=0, y=2),
    panels=[
        prewarming_pool_size,
        idle_runner,
        runner_startup_duration,
        used_runner
    ]
)
