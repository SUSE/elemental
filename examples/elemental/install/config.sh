#!/bin/bash

set -xe

# Disable Grub timeout
grub2-editenv /boot/grubenv set timeout=5
grub2-editenv /boot/grubenv set console=ttyS0,115200

# Setting root passwd
echo "uc0@linux" | passwd root --stdin

# Allow root ssh access (for testing purposes only!)
echo "PermitRootLogin yes" > /etc/ssh/sshd_config.d/root_access.conf
systemctl enable sshd
