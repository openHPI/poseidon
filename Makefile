PROJECT_NAME := "poseidon"
PKG := "gitlab.hpi.de/codeocean/codemoon/$(PROJECT_NAME)/cmd/$(PROJECT_NAME)"
UNIT_TESTS = $(shell go list ./... | grep -v /e2e)

DOCKER_E2E_CONTAINER_NAME := "$(PROJECT_NAME)-e2e-tests"
DOCKER_TAG := "poseidon:latest"
DOCKER_OPTS := -v $(shell pwd)/configuration.yaml:/configuration.yaml

default: help

.PHONY: all
all: build

.PHONY: bootstrap
bootstrap: deps lint-deps git-hooks ## Install all dependencies

.PHONY: deps
deps: ## Get the dependencies
	@go get -v -d ./...
	@go install github.com/vektra/mockery/v2@latest


.PHONY: git-hooks
git-hooks: .git/hooks/pre-commit ## Install the git-hooks
.git/hooks/%: git_hooks/%
	cp $^ $@
	chmod 755 $@

.PHONY: build
build: deps ## Build the binary
	@go build -o $(PROJECT_NAME) -v $(PKG)

.PHONY: clean
clean: ## Remove previous build
	@rm -f poseidon

.PHONY: docker
docker:
	@CGO_ENABLED=0 make build
	@docker build -t $(DOCKER_TAG) -f deploy/poseidon/Dockerfile .

.PHONY: lint-deps
lint-deps: ## Install linter dependencies
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: golangci-lint
golangci-lint: ## Lint the source code using golangci-lint
	@golangci-lint run ./... --timeout=3m

.PHONY: lint
lint: golangci-lint ## Lint the source code using all linters

.PHONY: mock
snaked_name=$(shell sed -e "s/\([a-z]\)\([A-Z]\)/\1_\2/g" -e "s/\([A-Z]\)/\L\1/g" -e "s/^_//" <<< "$(name)")
mock: deps ## Create/Update a mock. Example: make mock name=apiQuerier pkg=./nomad
	@mockery \
      --name=$(name) \
      --structname=$(name)Mock \
      --filename=$(snaked_name)_mock.go \
      --inpackage \
      --srcpkg=$(pkg)

.PHONY: test
test: deps ## Run unit tests
	@go test -count=1 -short $(UNIT_TESTS)

.PHONY: race
race: deps ## Run data race detector
	@go test -race -count=1 -short $(UNIT_TESTS)

.PHONY: coverage
coverage: deps ## Generate code coverage report
	@go test $(UNIT_TESTS) -v -coverprofile coverage.cov
	# exclude mock files from coverage
	@cat coverage.cov | grep -v _mock.go > coverage_cleaned.cov || true
	@go tool cover -func=coverage_cleaned.cov

.PHONY: coverhtml
coverhtml: coverage ## Generate HTML coverage report
	@go tool cover -html=coverage_cleaned.cov -o coverage_unit.html

.PHONY: e2e-test
e2e-test: deps ## Run e2e tests
	@go test -count=1 ./tests/e2e -v -args -dockerImage="drp.codemoon.xopic.de/openhpi/co_execenv_python:3.8"

.PHONY: e2e-docker
e2e-docker: docker ## Run e2e tests against the Docker container
	docker run --rm -p 127.0.0.1:7200:7200 \
       --name $(DOCKER_E2E_CONTAINER_NAME) \
       -e POSEIDON_SERVER_ADDRESS=0.0.0.0 \
       $(DOCKER_OPTS) \
       $(DOCKER_TAG) &
	@timeout 30s bash -c "until curl -s -o /dev/null http://127.0.0.1:7200/; do sleep 0.1; done"
	@make e2e-test || EXIT=$$?; docker stop $(DOCKER_E2E_CONTAINER_NAME); exit $$EXIT

# See https://aquasecurity.github.io/trivy/v0.18.1/integrations/gitlab-ci/
TRIVY_VERSION = $(shell wget -qO - "https://api.github.com/repos/aquasecurity/trivy/releases/latest" | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\1/')
.trivy/trivy:
	@mkdir -p .trivy
	@wget --no-verbose https://github.com/aquasecurity/trivy/releases/download/v$(TRIVY_VERSION)/trivy_$(TRIVY_VERSION)_Linux-64bit.tar.gz -O - | tar -zxvf - -C .trivy
	@chmod +x .trivy/trivy

# trivy only comes with a template for container_scanning but we want dependency_scanning here
.trivy/contrib/gitlab-dep.tpl: .trivy/trivy
	@sed -e "s/container_scanning/dependency_scanning/" .trivy/contrib/gitlab.tpl > $@

.PHONY: trivy-scan-deps
trivy-scan-deps: poseidon .trivy/contrib/gitlab-dep.tpl ## Run trivy vulnerability against our dependencies
	make trivy TRIVY_COMMAND="fs" TRIVY_TARGET="--skip-dirs .trivy --skip-files go.sum ." TRIVY_TEMPLATE="@.trivy/contrib/gitlab-dep.tpl"

.PHONY: trivy-scan-docker
trivy-scan-docker: ## Run trivy vulnerability scanner against the docker image
	make trivy TRIVY_COMMAND="i" TRIVY_TARGET="--skip-files home/api/poseidon $(DOCKER_TAG)" TRIVY_TEMPLATE="@.trivy/contrib/gitlab.tpl"

.PHONY: trivy
trivy: .trivy/trivy
	# Build report
	@.trivy/trivy --cache-dir .trivy/.trivycache/ $(TRIVY_COMMAND) --exit-code 0 --no-progress --format template --template $(TRIVY_TEMPLATE) -o .trivy/gl-scanning-report.json $(TRIVY_TARGET)
	# Print report
	@.trivy/trivy --cache-dir .trivy/.trivycache/ $(TRIVY_COMMAND) --exit-code 0 --no-progress $(TRIVY_TARGET)
	# Fail on severe vulnerabilities
	@.trivy/trivy --cache-dir .trivy/.trivycache/ $(TRIVY_COMMAND) --exit-code 1 --severity HIGH,CRITICAL --no-progress $(TRIVY_TARGET)

.PHONY: help
HELP_FORMAT="    \033[36m%-25s\033[0m %s\n"
help: ## Display this help screen
	@echo "Valid targets:"
	@grep -E '^[^ ]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; \
			{printf $(HELP_FORMAT), $$1, $$2}'
