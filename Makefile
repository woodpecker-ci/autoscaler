GO_PACKAGES ?= $(shell go list ./... | grep -v /vendor/)

TARGETOS ?= linux
TARGETARCH ?= amd64

LDFLAGS := -s -w -extldflags "-static"
CGO_ENABLED := 0

HAS_GO = $(shell hash go > /dev/null 2>&1 && echo "GO" || echo "NOGO" )
ifeq ($(HAS_GO),GO)
	CGO_CFLAGS ?= $(shell go env CGO_CFLAGS)
endif
CGO_CFLAGS ?=

# If the first argument is "in_docker"...
ifeq (in_docker,$(firstword $(MAKECMDGOALS)))
  # use the rest as arguments for "in_docker"
  MAKE_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  # Ignore the next args
  $(eval $(MAKE_ARGS):;@:)

  in_docker:
	@[ "1" -eq "$(shell docker image ls woodpecker/make:local -a | wc -l)" ] && docker buildx build -f ./docker/Dockerfile.make -t woodpecker/make:local --load . || echo reuse existing docker image
	@echo run in docker:
	@docker run -it \
		--user $(shell id -u):$(shell id -g) \
		-e CI_COMMIT_SHA="$(CI_COMMIT_SHA)" \
		-e TARGETOS="$(TARGETOS)" \
		-e TARGETARCH="$(TARGETARCH)" \
		-e CGO_ENABLED="$(CGO_ENABLED)" \
		-e GOPATH=/tmp/go \
		-e HOME=/tmp/home \
		-v $(PWD):/build --rm woodpecker/make:local make $(MAKE_ARGS)
else

# Proceed with normal make

##@ General

.PHONY: all
all: help

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

format: install-tools ## Format source code
	@gofumpt -extra -w .

.PHONY: clean
clean: ## Clean build artifacts
	go clean -i ./...
	rm -rf build
	@[ "1" != "$(shell docker image ls woodpecker/make:local -a | wc -l)" ] && docker image rm woodpecker/make:local || echo no docker image to clean


install-tools: ## Install development tools
	@hash golangci-lint > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; \
	fi ; \
	hash lint > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		go install github.com/rs/zerolog/cmd/lint@latest; \
	fi ; \
	hash gofumpt > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		go install mvdan.cc/gofumpt@latest; \
	fi ; \
	hash mockery > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		go install github.com/vektra/mockery/v2@latest; \
	fi ; \

##@ Test

.PHONY: lint
lint: install-tools ## Lint code
	@echo "Running golangci-lint"
	golangci-lint run
	@echo "Running zerolog linter"
	lint go.woodpecker-ci.org/autoscaler/cmd/woodpecker-autoscaler

test-autoscaler: ## Test autoscaler code
	go test -race -cover -coverprofile autoscaler-coverage.out -timeout 30s ${GO_PACKAGES}

.PHONY: test
test: test-autoscaler ## Run all tests

.PHONY: generate
generate:
	mockery

##@ Build

build:
	CGO_ENABLED=${CGO_ENABLED} GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags '${LDFLAGS}' -o dist/woodpecker-autoscaler go.woodpecker-ci.org/autoscaler/cmd/woodpecker-autoscaler

endif
