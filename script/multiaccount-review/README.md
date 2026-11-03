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

Run each scenario against its PR endpoint rather than an installed release or
another layer. The reset/downgrade scenario also requires an explicit path to an
older `gh`. It copies both executables into its fixture and does not replace your
installation.

## Isolation and limitations

The scenarios require macOS, Bash, Git, and `/usr/bin/sandbox-exec`. Some scenarios
also use `jq`. They refuse to run without OS containment. No dependencies are
installed automatically.

Only synthetic accounts and `.invalid` hosts are used by these scripts. Network
access, native Keychain access, and execution of `/usr/bin/security` are denied.
Git, gh and SSH configuration lives in private fixture directories outside the
checkout. Existing fixture directories are rejected rather than overwritten.

Where a scenario exercises Git transport, the server or SSH connection is
simulated locally. The real CLI and Git processes run, but this does not prove
live GitHub authentication, SSH key ownership, or enterprise-policy compatibility.
Profile email lookups and live resource creation are not part of these scripts.

Some scenarios retain their private fixture for inspection. Follow the specific
script's phase and cleanup instructions. Do not remove the sandbox or replace
synthetic hosts and credentials with real ones.

## Recordings

The PR descriptions contain separate terminal recordings of the product code at
each layer. The recordings show typed commands and their observed output, not
invocations of these scenario scripts.

Some recordings use real accounts with isolated credentials for read-only API
calls. The live SSH recordings additionally use temporary keys and private
personal-namespace repositories, which were removed after recording. Config
walkthroughs show the generated Git files at the corresponding stack layer.

These offline scripts are supplementary replay tools, not reproductions of every
recorded live interaction. Generated media and credential snapshots are not
committed to this directory.
