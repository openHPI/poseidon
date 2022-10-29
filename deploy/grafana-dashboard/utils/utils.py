
def read_query(name):
    with open("queries/" + name + ".flux", 'r') as file:
        return file.read()
