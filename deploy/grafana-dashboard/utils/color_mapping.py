from utils.utils import deep_update_dict
import json


def color_mapping(name, color):
    return {
        "fieldConfig": {
            "overrides": [{
                "matcher": {
                    "id": "byName",
                    "options": name
                },
                "properties": [{
                    "id": "color",
                    "value": {
                        "fixedColor": color,
                        "mode": "fixed"
                    }
                }]
            }]
        }
    }


grey_all_mapping = color_mapping("all", "#4c4b5a")
color_mapping_environments = {}
colours = [
            "#E41A1C", "#377EB8", "#4DAF4A", "#984EA3", "#FF7F00", "#FFFF33", "#A65628", "#F781BF", "#999999", # R ColorBrewer Set1
            "#8DD3C7", "#FFFFB3", "#BEBADA", "#FB8072", "#80B1D3", "#FDB462", "#B3DE69", "#FCCDE5", "#D9D9D9", "#BC80BD", "#CCEBC5", "#FFED6F" # R ColorBrewer Set3
]

with open("environments.json") as f:
    environments = json.load(f)

    environment_identifier = []
    for environment in environments["executionEnvironments"]:
        environment_identifier.append(str(environment["id"]) + "/" + environment["image"].removeprefix("openhpi/co_execenv_"))

    environment_identifier.sort()
    for environment in environment_identifier:
        deep_update_dict(color_mapping_environments, color_mapping(environment, colours.pop(0)))
