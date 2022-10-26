from utils.utils import deep_update_dict
from functools import reduce


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


def add_color_mapping(mapping_dict, new_item):
    deep_update_dict(mapping_dict, color_mapping(new_item[0], new_item[1]))
    return mapping_dict


color_mapping_environments = reduce(add_color_mapping, [
    ("p10/java:8-antlr", "yellow"),
    ("p28/r:4", "blue"),
    ("p29/python:3.8", "orange"),
    ("p31/java:17", "red"),
    ("p33/openhpi/docker_exec_phusion", "purple"),
    ("p11/java:8-antlr", "pink"),
    ("p14/python:3.4", "brown"),
    ("p18/node:0.12", "black"),
    ("p22/python:3.4-rpi-web", "white"),
    ("p25/ruby:2.5", "gray"),
    ("p30/python:3.7-ml", "gold"),
], {})
