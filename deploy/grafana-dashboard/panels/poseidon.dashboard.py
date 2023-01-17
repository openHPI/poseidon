from grafanalib.core import Dashboard, Templating, Time

from panels.availability_row import availability_panels
from panels.general_row import general_panels
from panels.runner_insights_row import runner_insights_panels
from utils.variables import environment_variable

dashboard = Dashboard(
    title="Poseidon autogen",
    timezone="browser",
    panels=availability_panels + general_panels + runner_insights_panels,
    templating=Templating(list=[ environment_variable ]),
    editable=True,
    refresh="30s",
    time=Time("now-6h", "now"),
    uid="P21Bh1SVk",
    version=1,
).auto_panel_ids()
