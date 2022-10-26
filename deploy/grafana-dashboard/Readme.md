# Grafana Dashboard Deployment

## Grafanalib

We use [Grafanalib](https://github.com/weaveworks/grafanalib) for the definition of our dashboard.
You need to install the python package: `pip install grafanalib`.

### Generation

Generate the Grafana dashboard by running `main.py`.
The generated Json definition can be imported in the Grafana UI.

### Deployment

You can copy the generated json into the field under the dashboards setting -> "JSON Model".
The version number needs to match!
