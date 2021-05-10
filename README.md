# Poseidon

[![pipeline status](https://gitlab.hpi.de/codeocean/codemoon/poseidon/badges/main/pipeline.svg)](https://gitlab.hpi.de/codeocean/codemoon/poseidon/-/commits/main)
[![coverage report](https://gitlab.hpi.de/codeocean/codemoon/poseidon/badges/main/coverage.svg)](https://gitlab.hpi.de/codeocean/codemoon/poseidon/-/commits/main)

## Setup

If you haven't installed Go on your system yet, follow the [golang installation guide](https://golang.org/doc/install).

The project can be compiled using `go build`. This should create a binary which can then be executed.

Alternatively, the `go run .` command can be used to automatically compile and run the project.

To run the tests, use `go test ./...`.

### Docker

The CI builds a Docker image and pushes it to our Docker registry at `drp.codemoon.xopic.de`. In order to pull an image from the registry you have to login with `sudo docker login drp.codemoon.xopic.de`. Execute `sudo docker run -p 3000:3000 drp.codemoon.xopic.de/<image name>` to run the image locally. You can find the image name in the `dockerimage` CI job. You can then interact with the webserver on your local port 3000.

You can also build the Docker image locally by executing `docker build -t <image name> .` in the root directory of this project. It assumes that the Go binary is named `poseidon` and available in the project root (see [here](#setup)). You can then start a Docker container with `sudo docker run --rm -p 3000:3000 <image name>`.

### Linter

Right now we use two different linters in our CI. See their specific instructions for how to use them:

- [Golang CI Lint](https://golangci-lint.run/usage/install/#local-installation)
- [Golang Lint](https://github.com/golang/lint)

### Git hooks

The repository contains a git pre-commit hook which runs the go formatting tool gofmt to ensure the code is formatted properly before committing. To enable it, you have to copy the hook file (`git_hooks/pre-commit`) to the `.git/hooks/` directory of the repository.

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

For the OpenAPI 3.0 definition of the API Poseidon provides, see [`swagger.yaml`](docs/swagger.yaml).

### Authentication

⚠️ Don't use authentication without TLS enabled, as otherwise the token will be transmitted in clear text.

⚠ We encourage you to enable authentication for this API. If disabled, everyone with access to your API has also indirectly access to your Nomad cluster as this API uses it.

The API supports authentication via an HTTP header. To enable it, specify the `server.token` value in the `configuration.yaml` or the corresponding environment variable `POSEIDON_SERVER_TOKEN`.

Once configured, all requests to the API, except the `health` route require the configured token in the `X-Poseidon-Token` header.

An example `curl` command with the configured token being `SECRET` looks as follows:

```bash
$ curl -H "X-Poseidon-Token: SECRET" http://localhost:3000/api/v1/some-protected-route
```

### TLS

We highly encourage the use of TLS in this API to increase the security. To enable TLS, set `server.tls` or the corresponding environment variable to true and specify the `server.certfile` and `server.keyfile` options.

You can create a self-signed certificate to use with this API using the following command.

```shell
$ openssl req -x509 -nodes -newkey rsa:2048 -keyout server.rsa.key -out server.rsa.crt -days 3650
```

## Tests

As testing framework we use the [testify](https://github.com/stretchr/testify) toolkit. 

For e2e tests we provide a separate package. E2e tests require the connection to a Nomad cluster.

### Mocks

For mocks we use [mockery](https://github.com/vektra/mockery). To generate a mock, first navigate to the package the interface is defined in.
You can then create a mock for the interface of your choice by running

```bash
mockery \
  --name=<<interface_name>> \
  --structname=<<interface_name>>Mock \
  --filename=<<interface_name>>Mock.go \
  --output=<<relative_path_to_output_folder>> \
  --outpkg=<<package_name_of_mock>>
```
on a specific interface.

For example, for an interface called `ExecutorApi` in the package `nomad`, you might run

```bash
mockery \
  --name=ExecutorApi \
  --output='.' \
  --structname=ExecutorApiMock \
  --filename=ExecutorApiMock.go \
  --outpkg=nomad
```

Note that the default value for `--output` is `./mocks` and the default for `--outpkg` is `mocks`. This will create the mock in a `mocks` sub-folder. However, in some cases (if the mock implements private interface methods), it needs to be in the same package as the interface it is mocking.

If the interface changes, you can rerun this command.
