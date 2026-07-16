#!/bin/bash

set -euo pipefail

key_file=$(mktemp)
trap 'rm -f "$key_file"' EXIT

if [[ -z "${SSH_KEY:-}" ]]; then
  echo "ECS_KEY is not configured" >&2
  exit 1
fi

# GitHub secrets can arrive as PEM text, escaped text, or base64.
ssh_key=${SSH_KEY//\\r/$'\r'}
ssh_key=${ssh_key//\\n/$'\n'}
if [[ "$ssh_key" == \"*\" && "$ssh_key" == *\" ]]; then
  ssh_key=${ssh_key:1:${#ssh_key}-2}
elif [[ "$ssh_key" == \'*\' && "$ssh_key" == *\' ]]; then
  ssh_key=${ssh_key:1:${#ssh_key}-2}
fi

if [[ "$ssh_key" == *"-----BEGIN "* ]]; then
  printf '%s\n' "$ssh_key" | tr -d '\r' > "$key_file"
else
  if ! printf '%s' "$ssh_key" | base64 --decode > "$key_file" 2>/dev/null; then
    echo "ECS_KEY is neither a private-key file nor valid base64" >&2
    exit 1
  fi
fi
chmod 600 "$key_file"

key_error=$(ssh-keygen -y -f "$key_file" 2>&1 >/dev/null || true)
if [[ -n "$key_error" ]]; then
  if grep -qiE 'passphrase|encrypted' <<< "$key_error"; then
    echo "ECS_KEY is encrypted; store an unencrypted OpenSSH private key in the GitHub secret" >&2
  elif grep -qE 'BEGIN (PUBLIC KEY|SSH2 PUBLIC KEY)' "$ssh_key" || grep -qE '^ssh-(rsa|ed25519|ecdsa)' "$ssh_key"; then
    echo "ECS_KEY contains a public key; store the matching private key instead" >&2
  else
    echo "ECS_KEY is not a valid unencrypted OpenSSH private key" >&2
  fi
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