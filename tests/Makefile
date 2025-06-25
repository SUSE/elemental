GINKGO       ?= "github.com/onsi/ginkgo/v2/ginkgo"
GINKGO_ARGS  ?= -v --fail-fast -r --timeout=3h

.PHONY: prepare-test
prepare-test:
ifeq ("$(DISK)","")
	@echo "No disk image found, run 'make build-disk' first"
	@exit 1
endif
	@scripts/run_vm.sh start $(DISK)
	@echo "VM started from $(DISK)"

.PHONY: test-clean
test-clean: test-stop
	@scripts/run_vm.sh clean

.PHONY: test-stop
test-stop:
	@scripts/run_vm.sh stop

.PHONY: test-installer
test-installer: prepare-test
	VM_PID=$$(scripts/run_vm.sh vmpid) go run $(GINKGO) $(GINKGO_ARGS) ./tests/installer
