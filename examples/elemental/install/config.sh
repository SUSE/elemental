#!/bin/bash

set -xe

# Disable Grub timeout
grub2-editenv /boot/grubenv set timeout=5
grub2-editenv /boot/grubenv set console=ttyS0,115200

# Setting root passwd
echo "linux" | passwd root --stdin

# Allow root ssh access (for testing purposes only!)
echo "PermitRootLogin yes" > /etc/ssh/sshd_config.d/root_access.conf
systemctl enable sshd

# Static host-key (for testing purposes only!)
cat > /etc/ssh/ssh_host_ecdsa_key <<EOF
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAaAAAABNlY2RzYS
1zaGEyLW5pc3RwMjU2AAAACG5pc3RwMjU2AAAAQQQw5slj5JGbABTKEU9Ca7rLeZYom0mi
kPjpDxOw05Eg76gt0Ub6Tnc3JMxGIfA3meiUhGj+fF61tjbfcGu8TDzcAAAAqMKQaBbCkG
gWAAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBDDmyWPkkZsAFMoR
T0Jrust5liibSaKQ+OkPE7DTkSDvqC3RRvpOdzckzEYh8DeZ6JSEaP58XrW2Nt9wa7xMPN
wAAAAgYsNrbSuJR2TC3+h+0rthmq2uRhFrq7m0F9KZHF4gKuQAAAANZnJlbG9uQGF0b21p
YwECAw==
-----END OPENSSH PRIVATE KEY-----
EOF

cat > /etc/ssh/ssh_host_ecdsa_key.pub <<EOF
ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBDDmyWPkkZsAFMoRT0Jrust5liibSaKQ+OkPE7DTkSDvqC3RRvpOdzckzEYh8DeZ6JSEaP58XrW2Nt9wa7xMPNw= user@elemental-vm
EOF
