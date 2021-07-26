# Poseidon

[![pipeline status](https://gitlab.hpi.de/codeocean/codemoon/poseidon/badges/main/pipeline.svg)](https://gitlab.hpi.de/codeocean/codemoon/poseidon/-/commits/main)
[![coverage report](https://gitlab.hpi.de/codeocean/codemoon/poseidon/badges/main/coverage.svg)](https://gitlab.hpi.de/codeocean/codemoon/poseidon/-/commits/main)

## Setup

If you haven't installed Go on your system yet, follow the [golang installation guide](https://golang.org/doc/install).

To get your local setup going, run `make bootstrap`. It will install all required dependencies as well as setting up our git hooks. Run `make help` to get an overview of available make targets.

The project can be compiled using `make build`. This should create a binary which can then be executed.

Alternatively, the `go run ./cmd/poseidon` command can be used to automatically compile and run the project.

### Docker

The CI builds a Docker image and pushes it to our Docker registry at `drp.codemoon.xopic.de`. In order to pull an image from the registry you have to login with `sudo docker login drp.codemoon.xopic.de`. Execute `sudo docker run -p 7200:7200 drp.codemoon.xopic.de/<image name>` to run the image locally. You can find the image name in the `dockerimage` CI job. You can then interact with the webserver on your local port 7200.

You can also build the Docker image locally by executing `make docker` in the root directory of this project. It builds the binary first and a container with the tag `poseidon:latest` afterwards. You can then start a Docker container with `sudo docker run --rm -p 7200:7200 poseidon:latest`.

### Linter

To lint our source code and ensure a common code style amongst our codebase we use [Golang CI Lint](https://golangci-lint.run/usage/install/#local-installation) as a linter. Use `make lint` to execute it.

### Git hooks

The repository contains a git pre-commit hook which runs the go formatting tool gofmt to ensure the code is formatted properly before committing. To enable them, run `make git-hooks`.

## Configuration

The file `config/config.go` contains a configuration struct containing all possible configuration options for Poseidon. The file also defines default values for most of the configuration options.  
The options *can* be overridden with a yaml configuration file whose path can be configured with the flag `-config`. By default, Poseidon searches for `configuration.yaml` in the project root. `configuration.example.yaml` is an example for a configuration file and contains explanations for all options.  
The options *can* also be overridden by environment variables. Currently, only the Go types `string`, `int`, `bool` and `struct` (nested) are implemented. The name of the environment variable is constructed as follows: `POSEIDON_(<name of nested struct>_)*<name of field>` (all letters are uppercase).

The precedence of configuration possibilities is:

1. Environment variables
1. Configuration file
1. Default values

If a value is not specified, the value of the subsequent possibility is used.

### Example

- The default value for the `Port` (type `int`) field in the `Server` field (type `struct`) of the configuration is `7200`.
- This can be overwritten with the following `configuration.yaml`:

  ```yaml
  server:
    port: 4000
  ```

- This can again be overwritten by the environment variable `POSEIDON_SERVER_PORT`. This can be done with `export POSEIDON_SERVER_PORT=5000`.

### Documentation

For the OpenAPI 3.0 definition of the API Poseidon provides, see [`swagger.yaml`](api/swagger.yaml).

### Authentication

⚠️ Don't use authentication without TLS enabled, as otherwise the token will be transmitted in clear text.

#### Poseidon

⚠ We encourage you to enable authentication for this API. If disabled, everyone with access to your API has also indirectly access to your Nomad cluster as this API uses it.

The API supports authentication via an HTTP header. To enable it, specify the `server.token` value in the `configuration.yaml` or the corresponding environment variable `POSEIDON_SERVER_TOKEN`.

Once configured, all requests to the API, except the `health` route require the configured token in the `X-Poseidon-Token` header.

An example `curl` command with the configured token being `SECRET` looks as follows:

```bash
$ curl -H "X-Poseidon-Token: SECRET" http://localhost:7200/api/v1/some-protected-route
```

#### Nomad

⚠ Enabling access control in the Nomad cluster is also recommended, to avoid having unauthorized actors perform unwanted actions in the cluster. Instructions on setting up the cluster appropriately can be found in [the Nomad documentation](https://learn.hashicorp.com/collections/nomad/access-control).

Afterwards, it is recommended to create a specific [Access Policy](https://learn.hashicorp.com/tutorials/nomad/access-control-policies?in=nomad/access-control) for Poseidon with the minimal set of capabilities it needs for operating the cluster. A non-minimal example with complete permissions can be found [here](docs/poseidon_policy.hcl). Poseidon requires a corresponding [Access Token](https://learn.hashicorp.com/tutorials/nomad/access-control-tokens?in=nomad/access-control) to send commands to Nomad. A Token looks like this:

```text
Accessor ID  = 463d3216-dc16-570f-380c-a48f5d26d955
Secret ID    = ea1ac4c5-892b-0bcc-9fc5-5faeb5273a13
Name         = Poseidon access token
Type         = client
Global       = false
Policies     = [poseidon]
Create Time  = 2021-07-26 12:45:11.437786378 +0000 UTC
Create Index = 246238
Modify Index = 246238
```

The `Secret ID` of the Token needs to be specified as the value of `nomad.token` value in the `configuration.yaml` or the corresponding environment variable `POSEIDON_NOMAD_TOKEN`. It may also be required for authentication in the Nomad Web UI and for using the Nomad CLI on the Nomad hosts (where the token can be specified via the `NOMAD_TOKEN` environment variable).

Once configured, all requests to the Nomad API automatically contain a `X-Nomad-Token` header containing the token.

⚠ Make sure that no (overly permissive) `anonymous` access policy is present in the cluster after the policy for Poseidon has been added. Anyone can perform actions as specified by this special policy without authenticating!

### TLS

We highly encourage the use of TLS in this API to increase the security. To enable TLS, set `server.tls` or the corresponding environment variable to true and specify the `server.certfile` and `server.keyfile` options.

You can create a self-signed certificate to use with this API using the following command.

```shell
$ openssl req -x509 -nodes -newkey rsa:2048 -keyout server.rsa.key -out server.rsa.crt -days 3650
```

## Tests

As testing framework we use the [testify](https://github.com/stretchr/testify) toolkit. 

Run `make test` to run the unit tests.

### E2E

For e2e tests we provide a separate package. E2e tests require the connection to a Nomad cluster.
Run `make e2e-tests` to run the e2e tests. This requires Poseidon to be already running.
Instead, you can run `make e2e-docker` to run the API in a Docker container, and the e2e tests afterwards.
You can use the `DOCKER_OPTS` variable to add additional arguments to the Docker run command that runs the API. By default, it is set to `-v $(shell pwd)/configuraton.yaml:/configuration.yaml`, which means, your local configuration file is mapped to the container. If you don't want this, use the following command.

```shell
$ make e2e-docker DOCKER_OPTS=""
```

### Mocks

For mocks we use [mockery](https://github.com/vektra/mockery). You can create a mock for the interface of your choice by running

```bash
make mock name=INTERFACE_NAME pkg=./PATH/TO/PKG
```

on a specific interface.

For example, for an interface called `ExecutorApi` in the package `nomad`, you might run

```bash
make mock name=ExecutorApi pkg=./nomad
```

If the interface changes, you can rerun this command (deleting the mock file first to avoid errors may be necessary).

Mocks can also be generated by using mockery directly on a specific interface. To do this, first navigate to the package the interface is defined in. Then run

```bash
mockery \
  --name=<<interface_name>> \
  --structname=<<interface_name>>Mock \
  --filename=<<interface_name>>Mock.go \
  --inpackage
```

For example, for an interface called `ExecutorApi` in the package `nomad`, you might run

```bash
mockery \
--name=ExecutorApi \
--structname=ExecutorAPIMock \
--filename=ExecutorAPIMock.go \
--inpackage
```

Note that per default, the mocks are created in a `mocks` sub-folder. However, in some cases (if the mock implements private interface methods), it needs to be in the same package as the interface it is mocking. The `--inpackage` flag can be used to avoid creating it in a subdirectory.
