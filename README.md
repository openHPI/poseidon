# CoolCodeOceanNomadMiddleware

This project is actively seeking for a name. Have an idea? Add it to #1

## Setup

If you haven't installed Go on your system yet, follow the [golang installation guide](https://golang.org/doc/install).

The project can be compiled using `go build`. This should create a binary which can then be executed.

Alternatively, the `go run .` command can be used to automatically compile and run the project.

To run the tests, use `go test`.

### Linter

Right now we use two different linters in our CI. See their specific instructions for how to use them:

- [Golang CI Lint](https://golangci-lint.run/usage/install/#local-installation)
- [Golang Lint](https://github.com/golang/lint)

### Git hooks

The repository contains a git pre-commit hook which runs the go formatting tool gofmt to ensure the code is formatted properly before committing. To enable it, you have to copy the hook file (`git_hooks/pre-commit`) to the `.git/hooks/` directory of the repository.
