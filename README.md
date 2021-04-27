# Poseidon

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
