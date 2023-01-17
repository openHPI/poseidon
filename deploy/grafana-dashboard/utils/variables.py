from grafanalib.core import Template

from utils.utils import read_query

environment_variable = Template(
    dataSource="Flux",
    label="Environment IDs",
    name="environment_ids",
    query=read_query("environment-ids"),
    refresh=1,
    includeAll=True,
    multi=True,
    default="$__all",
)
