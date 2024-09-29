# Nomad Usage

Poseidon is an abstraction of the functionality provided by Nomad. In the following we will look at how Poseidon uses Nomad's functionality.

Nomad is structured in different levels of abstraction. Jobs are collected in namespaces. Each Job can contain several Task Groups. Each Task Group can contain several Tasks. Finally, Allocations map Task Groups to Nomad Clients. For more insights take a look at [the official description](https://www.nomadproject.io/docs/internals/architecture).
In our case, a Task is executed in a Docker container.

![Overview Poseidon-Nomad mapping](resources/OverviewPoseidonNomadMapping.png)

## Execution environments as template Jobs

Execution Environments are mapped to Nomad Jobs. In the following, we will call these Jobs `Template Jobs`.
The naming schema for Template Jobs is "template-\<execution-environment-id\>".

The last figure shows the structure in Nomad.
Each template Job contains a "config" Task Group including a "config" Task. This Task does not perform any computations but is used to store environment-specific attributes, such as the prewarming pool size.
In addition, the template Job contains a "default-group" Task Group with a "default-task" Task. In this Task, `sleep infinity` is executed so that the Task remains active and is ready for dynamic executions in the container.

As shown in the figure, the "config" Task Group has no Allocation, while the "default-group" has an Allocation.
This is because the "config" Task Group only stores information but does not execute anything.
In the "default-group" the user's code submissions are executed. Therefore, Nomad creates an Allocation that points the Task Group to a Nomad Client for execution.

## Runner as Nomad Jobs

As an abstraction of the execution engine, we use `Runner` as a description for Docker containers (currently used) or microVMs.
If a user requests a new runner, Poseidon duplicates the template Job of the corresponding environment.

When a user then executes their code, Poseidon copies the code into the container and executes it.

### Nomad Event Stream

We use the [Nomad Event Stream](https://developer.hashicorp.com/nomad/api-docs/events) to subscribe to Nomad Events.
We handle `Evaluation` and `Allocation` events.
`Evaluation` events are used to wait for an environment to be created when requested.
`Allocation` events are used to track the starts and stops of runners.
Because of the mapping of runners to Nomad Jobs, we could also monitor Nomad's `Job` events (registered, deregistered), however, due to historical reasons we listen to `Allocation` events.
These events behave similarly because each Job has at least one allocation to be executed in.
However, the `Allocation` events also contain information about restarts and reschedulings.
This increases the complexity of the event stream handling but allows us to identify OOM Killed runners and move a used runner to the idle runners once it got restarted or rescheduled.

### Nomad Restarts

When the Nomad Servers or Agents restart, running executions can be terminated.  
For agents, it depends on whether the runner allocation is placed on the restarted agent.
For servers, it depends both on the role - if the restarted server is the cluster leader - and on whether Poseidon is connected to the restarted server, e.g. due to DNS Resolving.
Poseidon can be connected to the server either for individual execution or for receiving the Nomad Event Stream.
The following table lists the behavior for restarts of Nomad Servers depending on its role and on whether Poseidon is connected to it (via DNS Resolution).

| Role     | DNS Resolves | | WebSocket Problem? | Event Stream Problem? | 
|----------|--------------|-|--------------------|-----------------------|
| Leader   | Yes          | | problematic        | problematic           |
| Leader   | No           | | problematic        | fine                  |
| Follower | Yes          | | problematic        | problematic           |
| Follower | No           | | fine               | fine                  |

Such restarts lead to problems with either individual WebSocket connections of executions or the Nomad Event Stream.
When the Nomad Event Stream is aborted, Poseidon tries to reestablish it. Once it succeeds in doing so, all environments and runners are recovered from Nomad.

In the case of Nomad Agent restarts, the WebSocket connection of a running execution aborts.
Furthermore, when also Docker of the Nomad Agent is restarted, the containers are recreated.
Poseidon captures such occurrences and uses the runner as clean and idle.

### systemd Relationship Nomad - Docker

We suggest to connect the Nomad and Docker systemd services through a systemd `PartOf` relationship.
In a systemd overwrite (`/etc/systemd/system/nomad.service.d/override.conf`), the `PartOf` relationship can be defined as follows:

```ini
# /etc/systemd/system/nomad.service.d/override.conf
[Unit]
PartOf = docker.service
```

This results in Nomad being restarted once Docker restarts, but not vice versa.
At the moment, this behavior is necessary so that Nomad can recreate the CNI network interfaces (see [#490](https://github.com/openHPI/poseidon/issues/490)).

## Prewarming

To reduce the response time in the process of claiming a runner, Poseidon creates a pool of runners that have been started in advance.
When a user requests a runner, a runner from this pool can be used.
In the background, a new runner is created, thus replenishing the pool.
By running in the background, the user does not have to wait as long as the runner needs to start.
The implementation of this concept can be seen in [the Runner Manager](/internal/runner/manager.go).

### Lifecycle

The prewarming pool is initiated when a new environment is requested/created according to the requested prewarming pool size.

Every change on the environment (resource constraints, prewarming pool size, network access) leads to the destruction of the environment including all used and idle runners (the prewarming pool). After that, the environment and its prewarming pool is re-created.

Other causes which lead to the destruction of the prewarming pool are the explicit deletion of the environment by using the API route or when the corresponding template job for a given enviornment is no longer available on Nomad but a force update is requested using the `GET /execution-environments/{id}?fetch=true` route. The issue described in the latter case should not occur in normal operation, but could arise from either manually deleting the template job, scheduling issues on Nomad or other unforseenable edge cases.  
