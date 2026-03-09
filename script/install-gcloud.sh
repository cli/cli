#!/bin/bash

# Install Google Cloud SDK
# This script downloads and installs the Google Cloud SDK

set -e

echo "Installing Google Cloud SDK..."
curl https://sdk.cloud.google.com | bash

echo "Google Cloud SDK installation complete!"
echo "Please restart your shell or run: exec -l $SHELL"
