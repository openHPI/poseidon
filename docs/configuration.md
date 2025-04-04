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

### systemd

Poseidon can be configured to run as a systemd service. Poseidon can optionally also be configured to use a systemd socket.
The use of systemd provides capabilities for managing Poseidon's state and zero downtime deployments.
Minimal examples for systemd configurations can be found in `.github/workflows/resources`.


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

Additionally, we provide a [secure-bridge](./resources/secure-bridge.conflist) configuration for the `containernetworking-plugins`. We highly recommend to use this configuration, as it will automatically configure an appropriate firewall and isolate your local network. Store the [secure-bridge](./resources/secure-bridge.conflist) in an (otherwise) empty folder and specify that folder in `/etc/nomad.d/client.hcl`:

```hcl
    cni_config_dir = "<path to folder with *.conflist>"
```

If the path is not set up correctly or with a different name, the placement of allocations will fail in Nomad: `Constraint missing network filtered [all] nodes`. Be sure to set the "dns" and "dns-search" options in `/etc/docker/daemon.json` with reasonable defaults, for example with those shown in our [example configuration for Docker](./resources/docker.daemon.json).

### Network range

The default subnet range for Docker containers can be adjusted.
This can be done both in the Docker daemon configuration and the CNI secure-bridge configuration.
Accordingly, every container using the secure-bridge will receive an IP of the CNI configuration.
Both subnet range configurations should not be overlapping.

An example configuration could use `10.151.0.0/20` for all containers without the CNI secure-bridge and `10.151.16.0/20`
for all containers using the CNI secure bridge.
This would grant 4096 IPs to both subnets and keep 14 network range blocks of the `10.151.0.0/16` network free for future use (e.g., in other CNI configs).

### Use gVisor as a sandbox

We recommend using gVisor as a sandbox for the execution environments. First, [install gVisor following the official documentation](https://gvisor.dev/docs/user_guide/install/) and second, adapt the `/etc/docker/daemon.json` with reasonable defaults as shown in our [example configuration for Docker](./resources/docker.daemon.json).

## Supported Docker Images

In general, any Docker image can be used as an execution environment. 

### Users

If the `privilegedExecution` flag is set to `true` during execution, no additional user is required. Otherwise, the following two requirements must be met:

- A non-privileged user called `user` needs to be present in the image. This user is used to execute the code.
- The Docker image needs to have a `/sbin/setuser` script allowing the execution of the user code as a non-root user, similar to `/usr/bin/su`.

### Executable Commands

In order to function properly, Poseidon expects the following commands to be available within the PATH:

- `cat`
- `env`
- `ls`
- `mkfifo`
- `rm`
- `bash` (not compatible with `sh` or `zsh`)
- `sleep`
- `tar` (including the `--absolute-names` option)
- `true`
- `unset`
- `whoami`

Tests need additional commands:

- `echo`
- `head`
- `id`
- `make`
- `tail`
