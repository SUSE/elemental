GINKGO?="github.com/onsi/ginkgo/v2/ginkgo"

PKG?=./pkg/... ./internal/...

.PHONY: unit-tests
unit-tests:
	go run $(GINKGO) --label-filter '!rootlesskit' --race --cover --coverpkg=github.com/suse/elemental/... --github-output -p -r ${PKG}
ifeq (, $(shell which rootlesskit))
	@echo "No rootlesskit utility found, not executing tests requiring it"
else
	@mv coverprofile.out coverprofile.out.bk
	rootlesskit go run $(GINKGO) --label-filter 'rootlesskit' --race --cover --coverpkg=github.com/suse/elemental/... --github-output -p -r ${PKG}
	@grep -v "mode: atomic" coverprofile.out >> coverprofile.out.bk
	@mv coverprofile.out.bk coverprofile.out
endif
