# Configuration

Poseidon can be configured to suit different use cases.


## Poseidon

Poseidon's configuration options are described in the sample configuration file.


## Nomad

As a subsystem of Poseidon, Nomad can and should also be configured accordingly.


### Scheduler

By default, Nomad uses a bin-packing scheduler. This places all Jobs on one host. In our case, a high load then leads to one Nomad client being fully utilised while the others remain mostly idle.
To mitigate the overload of a Nomad client, the ["spread" scheduler algorithm](https://www.nomadproject.io/api-docs/operator/scheduler#update-scheduler-configuration) should be used.