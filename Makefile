GINKGO?= "github.com/onsi/ginkgo/v2/ginkgo"

PKG:=./pkg/...

.PHONY: unit-tests
unit-tests:
	go run $(GINKGO) --race --cover --coverpkg=github.com/suse/elemental/... --github-output -p -r ${PKG}
	@sed -n -i '/\/mock\//!p' coverprofile.out
	@go tool cover -func=coverprofile.out | grep total | awk '{print $$3}' | xargs echo -e "\nCoverage excluding mocks: "
