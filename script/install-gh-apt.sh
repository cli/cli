#!/usr/bin/env bash
set -euo pipefail

# Install wget if missing
if ! command -v wget >/dev/null; then
  sudo apt update
  sudo apt install -y wget
fi

# Create keyring directory
sudo mkdir -p -m 755 /etc/apt/keyrings

# Download key
out=$(mktemp)
wget --https-only --secure-protocol=TLSv1_2 --fail -nv -O "$out" https://cli.github.com/packages/githubcli-archive-keyring.gpg
sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg < "$out" > /dev/null
rm -f "$out"
sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg

# Add sources.list entry
sudo mkdir -p -m 755 /etc/apt/sources.list.d
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
  | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null

# Update and install gh
sudo apt update
sudo apt install -y gh
