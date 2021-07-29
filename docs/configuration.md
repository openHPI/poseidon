# Configuration

Poseidon can be configured to suit different use cases.


## Poseidon

Poseidon's configuration options are described in the sample configuration file.


## Nomad

As a subsystem of Poseidon, Nomad can and should also be configured accordingly.


### Scheduler

By default, Nomad uses a bin-packing scheduler. This places all Jobs on one host. In our case, a high load then leads to one Nomad client being fully utilised while the others remain mostly idle.
To mitigate the overload of a Nomad client, the ["spread" scheduler algorithm](https://www.nomadproject.io/api-docs/operator/scheduler#update-scheduler-configuration) should be used.

### Maximum Connections per Client

By default, Nomad only allows 100 maximum concurrent connections per client. However, as Poseidon is a client, this would significantly impair and limit the performance of Poseidon. Thus, this limit should be disabled.

To do so, ensure the following configuration is set in your Nomad agents, for example by adding it to `/etc/nomad.d/base.hcl`:

```hcl
limits {
    http_max_conns_per_client = 0
}
```
