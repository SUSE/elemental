GINKGO?="github.com/onsi/ginkgo/v2/ginkgo"

PKG?=./pkg/...

.PHONY: unit-tests
unit-tests:
	go run $(GINKGO) --race --cover --coverpkg=github.com/suse/elemental/... --github-output -p -r ${PKG}
