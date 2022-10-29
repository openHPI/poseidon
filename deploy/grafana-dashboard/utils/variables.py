from grafanalib.core import Template

from utils.utils import read_query

stage_variable = Template(
    dataSource="Poseidon",
    label="Stage",
    name="stages",
    query=read_query("stages"),
    refresh=1,
    includeAll=True,
    multi=True,
    default="production"
)

environment_variable = Template(
    dataSource="Poseidon",
    label="Environment IDs",
    name="environment_ids",
    query=read_query("environment-ids"),
    refresh=1,
    includeAll=True,
    multi=True,
    default="$__all"
)
