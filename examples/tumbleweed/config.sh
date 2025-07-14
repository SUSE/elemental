#!/bin/bash -x

set -e

# Set root password
echo "linux" | passwd root --stdin

# Enabling services
systemctl enable NetworkManager.service
systemctl enable systemd-sysext.service