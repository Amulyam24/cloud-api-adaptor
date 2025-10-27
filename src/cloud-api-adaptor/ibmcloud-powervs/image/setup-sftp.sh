#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/../..
PODVM_DIR=${REPO_ROOT}/podvm

mkdir /media/cidata

sudo cp -a "${PODVM_DIR}"/addons/sftp/* /etc/systemd/system/

#systemctl disable 'run-kata\x2dcontainers.mount'

# Enable/disable services
# Change mount point of kata-containers to type tmpfs.
KATA_MOUNT_FILE=/etc/systemd/system/'run-kata\x2dcontainers.mount'

if grep -q '^Type=none' "$KATA_MOUNT_FILE" && grep -q '^Options=bind' "$KATA_MOUNT_FILE"; then
  sudo sed -i 's/^Type=none/Type=tmpfs/; s/^Options=bind/Options=mode=0755,uid=root,gid=root/' "$KATA_MOUNT_FILE"
  echo "Updated $KATA_MOUNT_FILE to Type=tmpfs and Options=mode=755"
fi

systemctl enable process-user-data.path
systemctl disable process-user-data.service
systemctl enable media-cidata.mount
systemctl enable reboot-watcher.path
systemctl enable sftp-dir.service

# sshd config
SSH_CONFIG="/etc/ssh/sshd_config"

# Backup existing sshd_config
cp "${SSH_CONFIG}" "/etc/ssh/sshd_config.bak.$(date +%F_%T)"

# Build new SSH config
cat >> "$SSH_CONFIG" <<EOF
# SSH configuration updated by script
# Only allow SFTP access for user $USER_NAME

EOF

if [[ "$DISABLE_SSH_LOGIN" == "true" ]]; then
cat >> "$SSH_CONFIG" <<EOF
# Disable all forms of login
PermitRootLogin no
PasswordAuthentication no
ChallengeResponseAuthentication no
UsePAM yes
PubkeyAuthentication yes

EOF
fi

if ! id ${USER_NAME} >/dev/null 2>&1; then
    echo "User ${USER_NAME} not found, creating new user"
    mkdir -p /home/${USER_NAME}
    useradd -r -s /sbin/nologin -d /home/${USER_NAME} ${USER_NAME}
fi

# needed?
chown -R ${USER_NAME}:${USER_NAME} /home/${USER_NAME}
# cloud-init is better to add keys, instead of hardcoding it

cat >> "$SSH_CONFIG" <<EOF
# Only allow SFTP subsystem, no shell access
Subsystem sftp internal-sftp

# Restrict $USER_NAME user to SFTP only with chroot
Match User $USER_NAME
    ForceCommand internal-sftp
    ChrootDirectory /media
    PermitTunnel no
    AllowAgentForwarding no
    AllowTcpForwarding no
    X11Forwarding no
EOF
