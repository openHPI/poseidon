PROJECT_NAME = poseidon
REPOSITORY_OWNER = openHPI
PKG = github.com/$(REPOSITORY_OWNER)/$(PROJECT_NAME)/cmd/$(PROJECT_NAME)
UNIT_TESTS = $(shell go list ./... | grep -v /e2e | grep -v /recovery)
GOCOVERDIR=coverage

# Define the PGO file to be used for the build
PGO_FILE = ./cmd/$(PROJECT_NAME)/default.pgo

# Docker options
DOCKER_TAG = poseidon:latest
DOCKER_OPTS = -v $(shell pwd)/configuration.yaml:/configuration.yaml
LOWER_REPOSITORY_OWNER = $(shell echo $(REPOSITORY_OWNER) | tr A-Z a-z)

# Define image to be used in e2e tests. Requires `make` to be available.
E2E_TEST_DOCKER_CONTAINER = co_execenv_java
E2E_TEST_DOCKER_TAG = 17
E2E_TEST_DOCKER_IMAGE = $(LOWER_REPOSITORY_OWNER)/$(E2E_TEST_DOCKER_CONTAINER):$(E2E_TEST_DOCKER_TAG)
# The base image of the e2e test image. This is used to build the base image as well.
E2E_TEST_BASE_CONTAINER := docker_exec_phusion
E2E_TEST_BASE_IMAGE = $(LOWER_REPOSITORY_OWNER)/$(E2E_TEST_BASE_CONTAINER)

default: help

.PHONY: all
all: build

.PHONY: bootstrap
bootstrap: deps lint-deps gci-deps git-hooks ## Install all dependencies

.PHONY: deps
deps: ## Get the dependencies
	@go get -v ./...
	@go install github.com/vektra/mockery/v2@latest

.PHONY: upgrade-deps
upgrade-deps: ## Upgrade the dependencies
	@go get -u -v ./...

.PHONY: tidy-deps
tidy-deps: ## Remove unused dependencies
	@go mod tidy


.PHONY: git-hooks
git-hooks: .git/hooks/pre-commit ## Install the git-hooks
.git/hooks/%: git_hooks/%
	cp $^ $@
	chmod 755 $@

.PHONY: build
build: deps ## Build the binary
ifneq ("$(wildcard $(PGO_FILE))","")
# PGO_FILE exists
	@go build -pgo=$(PGO_FILE) -ldflags "-X main.pgoEnabled=true" -o $(PROJECT_NAME) -v $(PKG)
else
# PGO_FILE does not exist
	@go build -o $(PROJECT_NAME) -v $(PKG)
endif

.PHONY: build-cover
build-cover: deps ## Build the binary and with coverage support for e2e-tests
	@go build -cover -o $(PROJECT_NAME) -v $(PKG)

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

.PHONY: gci-deps
gci-deps: ## Install gci dependencies
	@go install github.com/daixiang0/gci@latest

.PHONY: gci
gci: ## Apply GCI to sort imports#
	@gci write --skip-generated -s standard -s default ./

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
	@mkdir -p $(GOCOVERDIR)
	@go test $(UNIT_TESTS) -v -coverprofile $(GOCOVERDIR)/coverage_output.cov -covermode atomic
	# exclude mock files from coverage
	@cat $(GOCOVERDIR)/coverage_output.cov | grep -v _mock.go > $(GOCOVERDIR)/coverage.cov || true
	@rm $(GOCOVERDIR)/coverage_output.cov
	@go tool cover -func=$(GOCOVERDIR)/coverage.cov

.PHONY: coverhtml
coverhtml: coverage ## Generate HTML coverage report
	@go tool cover -html=$(GOCOVERDIR)/coverage.cov -o $(GOCOVERDIR)/coverage_unit.html

deploy/dockerfiles: ## Clone Dockerfiles repository
	@git clone git@github.com:$(REPOSITORY_OWNER)/dockerfiles.git deploy/dockerfiles

.PHONY: run-with-coverage
run-with-coverage: build-cover ## Run binary and capture code coverage (during e2e tests)
	@mkdir -p $(GOCOVERDIR)
	@GOCOVERDIR=$(GOCOVERDIR) ./$(PROJECT_NAME)

## This target uses `systemd-socket-activate` (only Linux) to create a systemd socket and makes it accessible to a new Poseidon execution.
.PHONY: run-with-socket
run-with-socket: build
	@systemd-socket-activate -l 7200 ./$(PROJECT_NAME)

.PHONY: convert-run-coverage
convert-run-coverage: ## Convert coverage data (created by `run-with-coverage`) to legacy text format
	@go tool covdata textfmt -i $(GOCOVERDIR) -o $(GOCOVERDIR)/coverage_run.cov
	@go tool cover -html=$(GOCOVERDIR)/coverage_run.cov -o $(GOCOVERDIR)/coverage_run.html

.PHONY: e2e-test-docker-image
e2e-test-docker-image: deploy/dockerfiles ## Build Docker image that is used in e2e tests
	@docker build -t $(E2E_TEST_BASE_IMAGE) deploy/dockerfiles/$(E2E_TEST_BASE_CONTAINER)
	@docker build -t $(E2E_TEST_DOCKER_IMAGE) deploy/dockerfiles/$(E2E_TEST_DOCKER_CONTAINER)/$(E2E_TEST_DOCKER_TAG)

.PHONY: e2e-test
e2e-test: deps ## Run e2e tests
	@[ -z "$(docker images -q $(E2E_TEST_DOCKER_IMAGE))" ] || docker pull $(E2E_TEST_DOCKER_IMAGE)
	@go test -count=1 ./tests/e2e -v -args -dockerImage="$(E2E_TEST_DOCKER_IMAGE)"

.PHONY: e2e-test-recovery
e2e-test-recovery: deps ## Run recovery e2e tests
	@go test -count=1 ./tests/recovery -v -args -dockerImage="$(E2E_TEST_DOCKER_IMAGE)"

.PHONY: e2e-docker
e2e-docker: docker ## Run e2e tests against the Docker container
	docker run --rm -p 127.0.0.1:7200:7200 \
       --name $(E2E_TEST_DOCKER_CONTAINER) \
       -e POSEIDON_SERVER_ADDRESS=0.0.0.0 \
       $(DOCKER_OPTS) \
       $(DOCKER_TAG) &
	@timeout 30s bash -c "until curl -s -o /dev/null http://127.0.0.1:7200/; do sleep 0.1; done"
	@make e2e-test || EXIT=$$?; docker stop $(E2E_TEST_DOCKER_CONTAINER); exit $$EXIT

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
