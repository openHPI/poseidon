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
colours = ["yellow", "blue", "orange", "red", "purple",
           "pink", "brown", "black", "white", "gray",
           "gold", "super-light-red", "dark-red", "dark-orange", "super-light-yellow",
           "super-light-green", "dark-green", "dark-blue", "super-light-purple", "super-light-blue"]

with open("environments.json") as f:
    environments = json.load(f)
    for environment in environments:
        deep_update_dict(color_mapping_environments, color_mapping(environment, colours.pop(0)))
