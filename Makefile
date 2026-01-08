# Directory of Makefile
export ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

GINKGO?="github.com/onsi/ginkgo/v2/ginkgo"

BUILD_DIR?=./build

PKG?=./pkg/... ./internal/...
COVER_PKG?=github.com/suse/elemental/...
INTEG_PKG?=./tests/integration/...

GO_MODULE?=$(shell go list -m)
# Exclude files in ./tests folder
GO_FILES=$(shell find ./ -path ./tests -prune -o -name '*.go' -not -name '*_test.go' -print)
GO_FILES+=./go.mod
GO_FILES+=./go.sum

GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --candidates=50 --abbrev=0 --tags 2>/dev/null || echo "v0.0.1" )
VERSION?=$(GIT_TAG)-g$(GIT_COMMIT_SHORT)

LDFLAGS:=-w -s
LDFLAGS+=-X "$(GO_MODULE)/internal/cli/cmd.version=$(GIT_TAG)"
LDFLAGS+=-X "$(GO_MODULE)/internal/cli/cmd.gitCommit=$(GIT_COMMIT)"

GO_BUILD_ARGS?=-ldflags '$(LDFLAGS)'

# Used to build the OS image
DOCKER?=docker
ELEMENTAL_IMAGE_REPO?=local/elemental-image
ifdef PLATFORM
ARCH=$(subst linux/,,$(PLATFORM))
else
ARCH?=$(shell uname -m)
endif
PLATFORM?=linux/$(ARCH)

# Use vendor directory if it exists
ifneq (,$(wildcard ./vendor))
	GO_BUILD_ARGS+=-mod=vendor
endif

ifneq (,$(GO_EXTRA_ARGS))
	GO_BUILD_ARGS+=$(GO_EXTRA_ARGS)
endif

# No verbose unit tests by default
ifneq (,$(VERBOSE))
	GO_RUN_ARGS+=-v
endif

# Include tests Makefile only if explicitly set
ifneq (,$(INTEGRATION_TESTS))
	include tests/Makefile
endif

# Default target
.PHONY: all
all: $(BUILD_DIR)/elemental3 $(BUILD_DIR)/elemental3ctl

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

$(BUILD_DIR)/elemental3: $(GO_FILES)
	go build $(GO_BUILD_ARGS) -o $@ ./cmd/elemental

$(BUILD_DIR)/elemental3ctl: $(GO_FILES)
	go build $(GO_BUILD_ARGS) -o $@ ./cmd/elemental3ctl

.PHONY: image
image: VALID_RUNNERS := runner-elemental3 runner-elemental3ctl
image:
	$(if $(filter $(RUNNER),$(VALID_RUNNERS)),,\
	  $(error Invalid RUNNER '$(RUNNER)'. Must be one of: $(VALID_RUNNERS)))
	$(DOCKER) build --platform $(PLATFORM) --target $(RUNNER) --tag $(ELEMENTAL_IMAGE_REPO):$(VERSION) .

.PHONY: unit-tests
unit-tests:
	go run $(GINKGO) --label-filter '!rootlesskit' --race --cover --coverpkg=$(COVER_PKG) --github-output -p -r $(GO_RUN_ARGS) ${PKG} || exit $$?
ifeq (, $(shell which rootlesskit 2>/dev/null))
	@echo "No rootlesskit utility found, not executing tests requiring it"
else
	@mv coverprofile.out coverprofile.out.bk
	rootlesskit go run $(GINKGO) --label-filter 'rootlesskit' --race --cover --coverpkg=$(COVER_PKG) --github-output -p -r $(GO_RUN_ARGS) ${PKG} || exit $$?
	@grep -v "mode: atomic" coverprofile.out >> coverprofile.out.bk
	@mv coverprofile.out.bk coverprofile.out
endif

.PHONY: clean
clean:
	@rm -rfv $(BUILD_DIR)
	@find . -type f -executable -name '*.test' -exec rm -f {} \+
