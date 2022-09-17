# Configuration

Poseidon can be configured to suit different use cases.


## Poseidon

The file `config/config.go` contains a configuration struct containing all possible configuration options for Poseidon. The file also defines default values for most of the configuration options.  
The options *can* be overridden with a yaml configuration file whose path can be configured with the flag `-config`. By default, Poseidon searches for `configuration.yaml` in the working directory. `configuration.example.yaml` is an example for a configuration file and contains explanations for all options. The keys of the options specified in the configuration file must be written in lowercase.  
The options *can* also be overridden by environment variables. Currently, only the Go types `string`, `int`, `bool` and `struct` (nested) are implemented. The name of the environment variable is constructed as follows: `POSEIDON_(<name of nested struct>_)*<name of field>` (all letters are uppercase).

The precedence of configuration possibilities is:

1. Environment variables
2. Configuration file
3. Default values

If a value is not specified, the value of the subsequent possibility is used.

### Example

- The default value for the `Port` (type `int`) field in the `Server` field (type `struct`) of the configuration is `7200`.
- This can be overwritten with the following `configuration.yaml`:

  ```yaml
  server:
    port: 4000
  ```

- Again, this can be overwritten by the environment variable `POSEIDON_SERVER_PORT`, e.g., using `export POSEIDON_SERVER_PORT=5000`.


## Nomad

As a subsystem of Poseidon, Nomad can and should also be configured accordingly.

### Memory Oversubscription

Poseidon is using Nomad's feature of memory oversubscription. This way all Runner are allocated with just 16MB. The memory limit defined per execution environment is used as an upper bound for the memory oversubscription.  
On the one hand, this feature allows Nomad to execute much more Runner in parallel but, on the other hand, it introduces a risk of overloading the Nomad host. Still, this feature is obligatory for Poseidon to work and therefore needs to be enabled. [Example Configuration](./resources/server.example.hcl)

```hcl
default_scheduler_config {
    memory_oversubscription_enabled = true
}
```


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

### Enable Networking Support in Nomad

In order to allow full networking support in Nomad, the `containernetworking-plugins` are required on the host. They can be either installed manually or through a package manager. In the latter case, the installation path might differ. Hence, add the following line to the `client` directive of the Nomad configuration in `/etc/nomad.d/client.hcl`:

```hcl
    cni_path = "/usr/lib/cni"
```

If the path is not set up correctly or the dependency is missing, the following error will be shown in Nomad: `failed to find plugin "bridge" in path [/opt/cni/bin]`

### Use gVisor as a sandbox

We recommend using gVisor as a sandbox for the execution environments. First, [install gVisor following the official documentation](https://gvisor.dev/docs/user_guide/install/) and second, adapt the `/etc/docker/daemon.json` with reasonable defaults as shown in our [example configuration for Docker](./resources/docker.daemon.json).
