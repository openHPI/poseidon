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

The file `config.go` contains a configuration struct containing all possible configuration options for Poseidon. The file also defines default values for all configuration options.  
The options can be overridden with a `configuration.yaml` configuration file in the project root. `configuration.yaml.example` is an example for a configuration file.  
The options can also be overridden by environment variables. Currently, only the Go types `string`, `int`, `bool` and `struct` (nesting is possible) are implemented. The name of the environment variable is constructed as follows: `POSEIDON_(<name of nested struct>_)*<name of field>` (all letters are uppercase).

The precedence of configuration possibilities is:

1. Environment variables
1. `configuration.yaml`
1. Default values

If a value is not specified, the value of the subsequent possibility is used.

### Example

- The default value for the `Port` (type `int`) field in the `Server` field (type `struct`) of the configuration is `3000`.
- This can be overwritten with the following `configuration.yaml`:

  ```yaml
  server:
    port: 4000
  ```

- This can again be overwritten by the environment variable `POSEIDON_SERVER_PORT`. This can be done with `export POSEIDON_SERVER_PORT=5000`.

### Documentation

For the OpenAPI 3.0 definition of the API Poseidon provides, see [`swagger.yaml`](docs/swagger.yaml).

### TLS

We highly encourage the use of TLS in this API to increase the security. To enable TLS, set `server.tls` or the corresponding environment variable to true and specify the `server.certfile` and `server.keyfile` options.

You can create a self-signed certificate to use with this API using the following command.

```shell
$ openssl req -x509 -nodes -newkey rsa:2048 -keyout server.rsa.key -out server.rsa.crt -days 3650
```

## Tests

As testing framework we use the [testify](https://github.com/stretchr/testify) toolkit.  
For mocks we use [mockery](https://github.com/vektra/mockery).
With Mockery, you can create mocks by running `mockery -r --name=<<interface_name>>` on a specific interface.
If the interface changes, you can rerun this command.  
For e2e tests we provide a separate package. E2e tests require the connection to a Nomad cluster.
