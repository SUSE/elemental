ARG GO_VERSION=1.25

FROM --platform=$BUILDPLATFORM registry.opensuse.org/opensuse/bci/golang:${GO_VERSION} AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /work

# Add specific dirs to the image so cache is not invalidated when modifying non go files
ADD go.mod .
ADD go.sum .
RUN go mod download
ADD cmd cmd
ADD internal internal
ADD pkg pkg
ADD Makefile .
ADD .git .
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH make all

FROM registry.opensuse.org/opensuse/tumbleweed:latest AS runner-base

ARG TARGETARCH
RUN ARCH=$(uname -m); \
    [[ "${ARCH}" == "aarch64" ]] && ARCH="arm64"; \
    zypper --non-interactive removerepo repo-update || true; \
    zypper --non-interactive install --no-recommends xfsprogs \
        util-linux-systemd \
        e2fsprogs \
        udev \
        rsync \
        grub2 \
        dosfstools \
        grub2-${ARCH}-efi \
        mtools \
        gptfdisk \
        patterns-microos-selinux \
        btrfsprogs \
        btrfsmaintenance \
        snapper \
        lvm2 && \
    zypper clean --all

COPY --from=builder /work/build/elemental3ctl /usr/bin/elemental3ctl
COPY --from=builder /work/build/elemental3 /usr/bin/elemental3

FROM runner-base AS runner-elemental3ctl
ENTRYPOINT ["/usr/bin/elemental3ctl"]

FROM runner-base AS runner-elemental3

RUN zypper --non-interactive removerepo repo-update || true; \
    zypper --non-interactive install --no-recommends xorriso && \
    zypper clean --all

ENTRYPOINT ["/usr/bin/elemental3"]
