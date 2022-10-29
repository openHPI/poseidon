from grafanalib.core import Dashboard, Templating, Time

from panels.availability_row import availability_row
from panels.general_row import general_row
from panels.runner_insights_row import runner_insights_row
from utils.variables import stage_variable, environment_variable

dashboard = Dashboard(
    title="Poseidon autogen",
    timezone="browser",
    panels=[
        general_row,
        runner_insights_row,
        availability_row
    ],
    templating=Templating(list=[
        stage_variable,
        environment_variable
    ]),
    editable=True,
    refresh="30s",
    time=Time('now-6h', 'now'),
    uid="P21Bh1SVk",
    version=1
).auto_panel_ids()
