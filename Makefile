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
DISKSIZE?=20G
OS_REPO?=registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default
OS_VERSION?=latest
ELEMENTAL_IMAGE_REPO?=local/elemental-image
DOCKER_SOCK?=/var/run/docker.sock
ifdef PLATFORM
ARCH=$(subst linux/,,$(PLATFORM))
else
ARCH?=$(shell uname -m)
endif
PLATFORM?=linux/$(ARCH)
IMG?=$(BUILD_DIR)/elemental-os-image-$(ARCH)

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
	DISK?=$(realpath $(IMG).qcow2)
	include tests/Makefile
endif

# Use the same shell for all commands in a target, useful for the build mainly
.ONESHELL:

# Default target
.PHONY: all
all: $(BUILD_DIR)/elemental3 $(BUILD_DIR)/elemental3-toolkit

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

$(BUILD_DIR)/elemental3: $(GO_FILES)
	go build $(GO_BUILD_ARGS) -o $@ ./cmd/elemental

$(BUILD_DIR)/elemental3-toolkit: $(GO_FILES)
	go build $(GO_BUILD_ARGS) -o $@ ./cmd/elemental-toolkit

.PHONY: elemental-image
elemental-image:
	$(DOCKER) build --platform $(PLATFORM) --target runner --tag $(ELEMENTAL_IMAGE_REPO):$(VERSION) .

.PHONY: build-disk
build-disk: $(BUILD_DIR) elemental-image
	qemu-img create -f raw $(IMG).raw $(DISKSIZE)
	TARGET=$$(sudo losetup -f --show $(IMG).raw)
	cp examples/elemental/install/config.sh $(BUILD_DIR)
	$(DOCKER) run --rm \
		--volume $(DOCKER_SOCK):$(DOCKER_SOCK) \
		--volume $(BUILD_DIR):/build \
		--volume /dev:/dev \
		--volume /run/udev:/run/udev:ro \
		--privileged \
		$(ELEMENTAL_IMAGE_REPO):$(VERSION) \
		--debug install --os-image $(OS_REPO):$(OS_VERSION) --target $${TARGET} --cmdline "console=ttyS0,115200" --config /build/config.sh
	BUILD_ERR=$$?
	test $${BUILD_ERR} -eq 0 && qemu-img convert -c -p -O qcow2 $(IMG).raw $(IMG).qcow2
	sudo losetup -d $${TARGET}
	exit $${BUILD_ERR}

.PHONY: unit-tests
unit-tests:
	go run $(GINKGO) --label-filter '!rootlesskit' --race --cover --coverpkg=$(COVER_PKG) --github-output -p -r $(GO_RUN_ARGS) ${PKG}
ifeq (, $(shell which rootlesskit 2>/dev/null))
	@echo "No rootlesskit utility found, not executing tests requiring it"
else
	@mv coverprofile.out coverprofile.out.bk
	rootlesskit go run $(GINKGO) --label-filter 'rootlesskit' --race --cover --coverpkg=$(COVER_PKG) --github-output -p -r $(GO_RUN_ARGS) ${PKG}
	@grep -v "mode: atomic" coverprofile.out >> coverprofile.out.bk
	@mv coverprofile.out.bk coverprofile.out
endif

.PHONY: clean
clean:
	@rm -rfv $(BUILD_DIR)
	@find . -type f -executable -name '*.test' -exec rm -f {} \+
