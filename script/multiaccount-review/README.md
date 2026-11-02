# Multi-account review scenarios

These scenarios accompany the draft multi-account PR stack. Each layer adds a
scenario for the behaviour introduced in that layer. They are review tools,
not a setup guide for your own accounts.

## Run a scenario

Build the checked-out layer, then pass the absolute path to its executable:

```sh
make
env -i HOME="$HOME" PATH=/usr/bin:/bin:/opt/homebrew/bin \
  /bin/bash script/multiaccount-review/01-account.sh --gh "$PWD/bin/gh"
```

Each PR description supplies the command for its scenario. Run the scenario
against that PR's endpoint rather than an installed release or another layer.
The reset/downgrade scenario also requires an explicit path to an older `gh`.
It copies both executables into its fixture and does not replace your installation.

## Isolation and limitations

The scenarios require macOS, Bash, Git, and `/usr/bin/sandbox-exec`. Some scenarios
also use `jq`. They refuse to run without the OS containment mechanism. No
dependencies are installed automatically.

Only synthetic accounts and `.invalid` hosts are used. Network access, native
Keychain access, and execution of `/usr/bin/security` are denied. Git, gh and SSH
configuration lives in private fixture directories outside the checkout.
Supplied existing fixture directories are rejected rather than overwritten.

Where a scenario exercises Git transport, the server or SSH connection is
simulated locally. The real CLI and Git processes run, but this does not prove
live GitHub authentication, SSH key ownership, or enterprise-policy compatibility.
Profile email lookups and live resource creation are not part of these scripts.

Some scenarios retain their private fixture for inspection. Follow the specific
script's phase and cleanup instructions. Do not remove the sandbox or replace
synthetic hosts and credentials with real ones.

## Recordings

PR videos record preparation, the exercised command, and validation in sequence.
Recordings use the product code at the corresponding layer. Scenario packaging
was subsequently tightened to reject unsafe fixture roots, and the delivered
scripts were replayed separately. Raw captures were not edited to imply those
packaging changes were part of the original recording.

The recordings and their raw capture files are not committed to this directory.
