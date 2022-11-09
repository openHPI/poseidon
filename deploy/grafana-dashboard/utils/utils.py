def read_query(*names):
    result = ""
    for name in names:
        with open("queries/" + name + ".flux", "r") as file:
            result += file.read()
    return result
