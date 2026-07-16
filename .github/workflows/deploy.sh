#!/bin/bash
set -e

# Create SSH key file with proper handling, stripping Windows-style line endings
printf '%s' "$SSH_KEY" | tr -d '\r' > key.pem
chmod 600 key.pem

# Verify key file was created properly
if [ ! -f key.pem ] || [ ! -s key.pem ]; then
  echo "Error: SSH key file not created or is empty"
  exit 1
fi

echo "Deploying to $SSH_USER@$SSH_HOST..."

# Copy binary and service file via SCP
scp -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=10 \
    -o BatchMode=yes \
    -o PasswordAuthentication=no \
    -i key.pem \
    unchain unchain.service \
    "${SSH_USER}@${SSH_HOST}:~"

# Execute deployment commands via SSH
ssh -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=10 \
    -o BatchMode=yes \
    -o PasswordAuthentication=no \
    -i key.pem \
    "${SSH_USER}@${SSH_HOST}" << 'SSHEOF'
  set -e
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
SSHEOF

# Clean up
rm -f key.pem
echo "Deployment completed successfully"
