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
