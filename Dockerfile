ARG GO_VERSION=1.24

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

ARG TARGETOS 
ARG TARGETARCH

RUN apk add --no-cache make git

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

FROM registry.opensuse.org/opensuse/tumbleweed:latest AS runner

ARG TARGETARCH

RUN ARCH=$(uname -m); \
    [[ "${ARCH}" == "aarch64" ]] && ARCH="arm64"; \
    zypper --non-interactive removerepo repo-update || true; \
    zypper install -y --no-recommends xfsprogs \
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
    zypper cc -a

COPY --from=builder /work/build/elemental3-toolkit /usr/bin/elemental3-toolkit
COPY --from=builder /work/build/elemental3 /usr/bin/elemental3

ENTRYPOINT ["/usr/bin/elemental3-toolkit"]
