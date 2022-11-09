def read_query(*names):
    result = ""
    for name in names:
        with open("queries/" + name + ".flux", "r") as file:
            result += file.read()
        result += "\n"
    return result


def deep_update_dict(base_dict, extra_dict):
    if extra_dict is None:
        return base_dict

    for k, v in extra_dict.items():
        update_dict_entry(base_dict, k, v)


def update_dict_entry(base_dict, k, v):
    if k in base_dict and hasattr(base_dict[k], "to_json_data"):
        base_dict[k] = base_dict[k].to_json_data()
    if k in base_dict and isinstance(base_dict[k], dict):
        deep_update_dict(base_dict[k], v)
    elif k in base_dict and isinstance(base_dict[k], list):
        base_dict[k].extend(v)
    else:
        base_dict[k] = v
