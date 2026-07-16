#!/bin/bash

set -euo pipefail

key_file=$(mktemp)
trap 'rm -f "$key_file"' EXIT

# GitHub secrets may contain literal escaped newlines when supplied by automation.
ssh_key=${SSH_KEY//\\n/$'\n'}
if [[ "$ssh_key" == *"-----BEGIN"* ]]; then
  printf '%s\n' "$ssh_key" | tr -d '\r' > "$key_file"
else
  printf '%s' "$ssh_key" | base64 --decode > "$key_file"
fi
chmod 600 "$key_file"

if ! ssh-keygen -y -f "$key_file" >/dev/null 2>&1; then
  echo "SSH_KEY is not a valid unencrypted private key" >&2
  exit 1
fi

ssh_options=(
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
  -o BatchMode=yes
  -o PasswordAuthentication=no
  -o ConnectTimeout=10
  -i "$key_file"
)
remote="${SSH_USER}@${SSH_HOST}"

scp "${ssh_options[@]}" unchain unchain.service "${remote}:~"
ssh "${ssh_options[@]}" "$remote" << EOF
  cd ~ && pwd
  sudo rm -rf /app && sudo mkdir /app
  sudo mv unchain /app/unchain
  sudo chmod +x /app/unchain
  echo "$CONFIG_TOML" | sudo tee /app/config.toml > /dev/null
  sudo mv unchain.service /etc/systemd/system/unchain.service
  sudo systemctl daemon-reload
  sudo systemctl stop unchain.service || true
  sudo systemctl start unchain.service
  sudo systemctl status unchain.service
EOF