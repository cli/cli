Fixes #13551

When an extension was already installed, running `gh extension install OWNER/REPO --pin <tag> --force` would silently route to `upgradeFunc`, which ignored the `--pin` argument entirely.
This PR fixes it by explicitly checking for the presence of `--pin` when upgrading an existing extension. If `--pin` is provided alongside `--force`, the CLI first removes the existing installation before reinstalling it at the requested pinned version, preventing the CLI from defaulting to the latest version.
