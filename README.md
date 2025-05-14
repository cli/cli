GitHub CLI (gh)
gh is GitHub on the command line. It brings pull requests, issues, and other GitHub concepts to the terminal, alongside where you are already working with git and your code.

Screenshot of gh pr status

Build Status Latest Release Downloads License

Table of Contents
Introduction
Features
Installation
macOS
Linux & BSD
Windows
Codespaces
GitHub Actions
Binary Verification
Development Workflows
Troubleshooting and Error Handling
Comparison with hub
Contributing
Introduction
GitHub CLI (gh) is supported for users on:

GitHub.com
GitHub Enterprise Cloud
GitHub Enterprise Server 2.20+
Platform Compatibility:

macOS
Windows
Linux
For detailed usage instructions, refer to the manual.

Features
Manage pull requests, issues, and repositories directly from your terminal.
Create, view, and merge pull requests effortlessly.
Automate workflows with scripting support.
Securely manage authentication with OAuth and tokens.
Built-in security with binary verification and provenance attestation.
Installation
macOS
Install using Homebrew:

bash
brew install gh
<details> <summary>Other Installation Methods</summary>
MacPorts: sudo port install gh
Conda: conda install gh --channel conda-forge
Spack: spack install gh
Webi: curl -sS https://webi.sh/gh | sh
Flox: flox install gh
</details>
Linux & BSD
Install via:

Debian and RPM repositories
Community-maintained repositories for various Linux distributions.
OS-agnostic package managers like Homebrew, Conda, Spack, and Webi.
Precompiled binaries from the releases page.
Windows
Install using:

Winget: winget install --id GitHub.cli
Scoop: scoop install gh
Chocolatey: choco install gh
Alternatively, download the MSI installer from the releases page.

Note: After installation, open a new terminal window to update your PATH.

Codespaces
Add GitHub CLI to your Codespace by including this snippet in your devcontainer.json:

JSON
"features": {
  "ghcr.io/devcontainers/features/github-cli:1": {}
}
GitHub Actions
GitHub CLI comes pre-installed in all GitHub-hosted runners.

Binary Verification
To ensure secure downloads, verify binaries using one of the following methods:

Using gh (if already installed):
bash
gh at verify -R cli/cli <filename>
Using Sigstore cosign:
bash
cosign verify-blob-attestation --bundle cli-cli-attestation-<version>.sigstore.json \
      --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
      --certificate-identity="https://github.com/cli/cli/.github/workflows/deployment.yml@refs/heads/trunk" \
      <filename>
Development Workflows
Setup
Clone the Repository:
bash
git clone https://github.com/cli/cli.git
cd cli
Install Dependencies: Ensure you have Go installed, then tidy up dependencies:
bash
go mod tidy
Build and Test
Build the CLI: Create a local binary:

bash
go build -o gh
Run Tests: Run all tests to ensure everything works as expected:

bash
go test ./...
Format Code: Ensure code is properly formatted:

bash
go fmt ./...
Contribute
Follow the Contributing Guide for detailed instructions on how to submit your changes.
Troubleshooting and Error Handling
Common Issues
Command Not Found: Ensure gh is in your PATH. Restart your terminal or run:
bash
export PATH=$PATH:/path/to/gh
Authentication Problems: Run the following to log in again:
bash
gh auth login
Permission Denied: For installation issues, try using sudo:
bash
sudo <command>
Debugging
Use the --verbose flag to get detailed output for any command:
bash
gh pr create --verbose
Comparison with hub
Feature	hub	gh
Proxy to git	✅	❌
Standalone Tool	❌	✅
Official GitHub CLI	❌	✅
For more details, see the comparison guide.

Contributing
We welcome contributions! Here's how you can get started:

Fork this repository.
Clone your fork locally:
bash
git clone https://github.com/<your-username>/cli.git
Create a new branch for your feature or fix:
bash
git checkout -b feature/your-feature-name
Make your changes and commit them:
bash
git commit -m "Add a detailed feature"
Push your changes and open a pull request.
For more details, see the Contributing Guide.

Final Notes
This draft includes all the requested updates:

Added badges for visibility.
Added detailed Development Workflows.
Added Troubleshooting and Error Handling section.
Kept the content focused on GitHub CLI.
