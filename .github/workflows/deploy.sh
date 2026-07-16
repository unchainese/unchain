#!/bin/bash


echo "$SSH_KEY" | tr -d '\r' > key.pem

chmod 600 key.pem

scp -o StrictHostKeyChecking=no -o BatchMode=yes -o PasswordAuthentication=no -i key.pem unchain unchain.service $SSH_USER@$SSH_HOST:~
ssh -o StrictHostKeyChecking=no -o BatchMode=yes -o PasswordAuthentication=no -i key.pem $SSH_USER@$SSH_HOST << EOF
  cd ~ && pwd
  sudo rm -rf /opt/unchain && sudo mkdir -p /opt/unchain
  sudo mv unchain /opt/unchain/app
  sudo chmod +x /opt/unchain/app
  echo "$ENV_VARS" | sudo tee /opt/unchain/.env > /dev/null
  sudo mv unchain.service /etc/systemd/system/unchain.service
  sudo systemctl daemon-reload
  sudo systemctl stop unchain.service || true
  sudo systemctl start unchain.service
  sudo systemctl status unchain.service
EOF