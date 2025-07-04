FROM registry.opensuse.org/opensuse/tumbleweed:latest AS builder

RUN zypper --non-interactive --gpg-auto-import-keys --installroot /target install --no-recommends -- \
      patterns-base-minimal_base \
      kernel-default \
      dbus-broker \
      device-mapper \
      dracut \
      grub2 \
      grub2-x86_64-efi \
      haveged \
      systemd \
      NetworkManager \
      openssh-server \
      openssh-clients \
      timezone \
      parted \
      e2fsprogs \
      dosfstools \
      mtools \
      xorriso \
      findutils \
      gptfdisk \
      rsync \
      squashfs \
      lvm2 \
      tar \
      gzip \
      vim \
      which \
      less \
      sudo \
      shim \
      curl \
      sed \
      iproute2 \
      btrfsprogs \
      btrfsmaintenance \
      snapper \
      xterm-resize

# make sure /boot is empty, elemental will install artifacts here.
RUN rm -rf /target/boot && mkdir /target/boot

FROM scratch
COPY --from=builder /target /

# Enable essential services
RUN systemctl enable NetworkManager.service && \
    systemctl enable sshd.service

# Workaround to make sure there are no pending sysusers to be created (boo#1231244)
RUN systemd-sysusers

# This is for automatic testing purposes, do not do this in production.
RUN echo "PermitRootLogin yes" > /etc/ssh/sshd_config.d/rootlogin.conf


# Generate initrd in /usr/lib/modules/*
RUN kver=$(ls /usr/lib/modules | head -n1) && \ 
    dracut -f --no-hostonly "/usr/lib/modules/${kver}/initrd" "${kver}"
