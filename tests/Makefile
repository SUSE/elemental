GINKGO       ?= github.com/onsi/ginkgo/v2/ginkgo
GINKGO_ARGS  ?= -v --fail-fast -r --timeout=3h

IMG_NAME?=elemental-os-image-$(ARCH)
IMG_REPO?=registry.opensuse.org/devel/unifiedcore/tumbleweed/containers

### Elemental install specific variables ###
IMG?=$(BUILD_DIR)/$(IMG_NAME)
DISK?=$(realpath $(IMG).qcow2)
DISKSIZE?=20G
OS_IMG?=$(IMG_REPO)/uc-base-os-kernel-default
OS_VERSION?=latest
DOCKER_SOCK?=/var/run/docker.sock

# Use the same shell for all commands in a target, useful for the build mainly
.ONESHELL:

.PHONY: build-disk
build-disk: RUNNER := runner-elemental3ctl
build-disk: $(BUILD_DIR) image
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
		--debug install --os-image $(OS_IMG):$(OS_VERSION) --target $${TARGET} --cmdline "console=ttyS0,115200" --config /build/config.sh
	BUILD_ERR=$$?
	test $${BUILD_ERR} -eq 0 && qemu-img convert -c -p -O qcow2 $(IMG).raw $(IMG).qcow2
	sudo losetup -d $${TARGET}
	exit $${BUILD_ERR}

.PHONY: test-clean
test-clean: test-stop
	@scripts/run_vm.sh clean

.PHONY: test-stop
test-stop:
	@scripts/run_vm.sh stop

.PHONY: test-installer
test-installer: build-disk
	@scripts/run_vm.sh start "$(DISK)"
	VM_PID=$$(scripts/run_vm.sh vmpid) go run $(GINKGO) $(GINKGO_ARGS) ./tests/installer
