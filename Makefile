GINKGO?="github.com/onsi/ginkgo/v2/ginkgo"

BUILD_DIR?=./build

PKG?=./pkg/... ./internal/...
COVER_PKG?=github.com/suse/elemental/...

GO_MODULE?=$(shell go list -m)
GO_FILES=$(shell find ./ -name '*.go' -not -name '*_test.go')
GO_FILES+=./go.mod
GO_FILES+=./go.sum

GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --candidates=50 --abbrev=0 --tags 2>/dev/null || echo "v0.0.1" )

LDFLAGS:=-w -s
LDFLAGS+=-X "$(GO_MODULE)/internal/cli/version.version=$(GIT_TAG)"
LDFLAGS+=-X "$(GO_MODULE)/internal/cli/version.gitCommit=$(GIT_COMMIT)"

GO_BUILD_ARGS?=-ldflags '$(LDFLAGS)'

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

.PHONY: all
all: $(BUILD_DIR)/elemental3 $(BUILD_DIR)/elemental3-toolkit

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

$(BUILD_DIR)/elemental3: $(GO_FILES)
	go build $(GO_BUILD_ARGS) -o $@ ./cmd/elemental

$(BUILD_DIR)/elemental3-toolkit: $(GO_FILES)
	go build $(GO_BUILD_ARGS) -o $@ ./cmd/elemental-toolkit

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
