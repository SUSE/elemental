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

### Elemental customize specific variables ###
RELEASE_MANIFEST?=registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/release-manifest:latest
CONFIG_DIR?=$(abspath ./tests/testdata/config-dir)
CUSTOMIZED_WORKDIR_NAME=customized
CUSTOMIZED_WORKDIR_PATH?=$(CONFIG_DIR)/$(CUSTOMIZED_WORKDIR_NAME)
CUSTOMIZED_IMG_MAC=FE:C4:05:42:8B:AB

$(CONFIG_DIR)/release.yaml:
	@echo "manifestURI: oci://$(RELEASE_MANIFEST)" > "$@"

define CUSTOMIZE
	$(DOCKER) run --rm \
		--volume "$(CONFIG_DIR):/config" \
		"$(ELEMENTAL_IMAGE_REPO):$(VERSION)" \
		--debug customize \
		--type "$(MEDIA_TYPE)" \
		--output "/config/$(CUSTOMIZED_WORKDIR_NAME)/$(IMG_NAME).$(MEDIA_TYPE)"
endef

# Use the same shell for all commands in a target, useful for the build mainly
.ONESHELL:
# Ensure an immediate exit when a command returns a non-zero status
.SHELLFLAGS := -ec

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

.PHONY: prepare-config-dir
prepare-config-dir: $(CONFIG_DIR)/release.yaml
	@mkdir -p "$(CUSTOMIZED_WORKDIR_PATH)"

.PHONY: customize-raw
customize-raw: RUNNER := runner-elemental3
customize-raw: MEDIA_TYPE := raw
customize-raw: image prepare-config-dir
	$(CUSTOMIZE)

.PHONY: customize-iso
customize-iso: RUNNER := runner-elemental3
customize-iso: MEDIA_TYPE := iso
customize-iso: image prepare-config-dir
	$(CUSTOMIZE)

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

.PHONY: test-customize-raw
test-customize-raw: customize-raw
	@ELMNTL_MAC=$(CUSTOMIZED_IMG_MAC) scripts/run_vm.sh start "$(CUSTOMIZED_WORKDIR_PATH)/$(IMG_NAME).raw"
	VM_PID=$$(scripts/run_vm.sh vmpid) go run $(GINKGO) $(GINKGO_ARGS) ./tests/customize

.PHONY: test-customize-iso
test-customize-iso: customize-iso
	@ELMNTL_MAC=$(CUSTOMIZED_IMG_MAC) scripts/run_vm.sh start "$(CUSTOMIZED_WORKDIR_PATH)/$(IMG_NAME).iso"
	VM_PID=$$(scripts/run_vm.sh vmpid) go run $(GINKGO) $(GINKGO_ARGS) ./tests/customize
