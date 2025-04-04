GINKGO?="github.com/onsi/ginkgo/v2/ginkgo"

PKG?=./pkg/... ./internal/...

GO_MODULE ?= $(shell go list -m)
GO_FILES  = $(shell find ./ -name '*.go' -not -name '*_test.go')

GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --candidates=50 --abbrev=0 --tags 2>/dev/null || echo "v0.0.1" )

LDFLAGS:=-w -s
LDFLAGS+=-X "$(GO_MODULE)/internal/cli/cmd.version=$(GIT_TAG)"
LDFLAGS+=-X "$(GO_MODULE)/internal/cli/cmd.gitCommit=$(GIT_COMMIT)"

# No verbose unit tests by default
ifeq ($(VERBOSE),true)
	VERBOSE_TEST?=-v
endif

elemental: $(GO_FILES)
	go build -ldflags '$(LDFLAGS)' -o $@ ./cmd/...

.PHONY: unit-tests
unit-tests:
	go run $(GINKGO) --label-filter '!rootlesskit' --race --cover --coverpkg=github.com/suse/elemental/... --github-output -p -r $(VERBOSE_TEST) ${PKG}
ifeq (, $(shell which rootlesskit 2>/dev/null))
	@echo "No rootlesskit utility found, not executing tests requiring it"
else
	@mv coverprofile.out coverprofile.out.bk
	rootlesskit go run $(GINKGO) --label-filter 'rootlesskit' --race --cover --coverpkg=github.com/suse/elemental/... --github-output -p -r $(VERBOSE_TEST) ${PKG}
	@grep -v "mode: atomic" coverprofile.out >> coverprofile.out.bk
	@mv coverprofile.out.bk coverprofile.out
endif
