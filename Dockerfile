ARG GO_VERSION=1.26
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
        cpio \
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
    zypper --non-interactive install --no-recommends xorriso helm && \
    zypper clean --all

ARG HAULER_VERSION=2.1.1
ADD https://github.com/hauler-dev/hauler/releases/download/v${HAULER_VERSION}/hauler_${HAULER_VERSION}_linux_amd64.tar.gz /tmp/hauler-amd64.tgz
ADD https://github.com/hauler-dev/hauler/releases/download/v${HAULER_VERSION}/hauler_${HAULER_VERSION}_linux_arm64.tar.gz /tmp/hauler-arm64.tgz
ARG TARGETARCH
RUN for arch in amd64 arm64; do mkdir -p /usr/lib/elemental/hauler/${arch} && tar -xzf /tmp/hauler-${arch}.tgz -C /usr/lib/elemental/hauler/${arch} hauler; done && \
    rm /tmp/hauler-*.tgz && \
    ln -s /usr/lib/elemental/hauler/${TARGETARCH}/hauler /usr/bin/hauler

ENTRYPOINT ["/usr/bin/elemental3"]
