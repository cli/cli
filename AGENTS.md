# Debloat goals

This fork tracks `cli/cli` and removes selected features to keep the GitHub CLI smaller and free of telemetry. Keep upstream behavior current outside the removals below.

## Expected changes from upstream

| Area | Upstream `cli/cli` | This fork |
| --- | --- | --- |
| Telemetry | Configuration, event recording, reporting, and telemetry commands | No telemetry configuration, collection, reporting, or commands |
| Prompts | Survey plus alternate Charm and accessibility prompt implementations | Survey prompts only |
| Extensions | Install, manage, discover, and run extensions | No extension system or extension commands |
| Attestations | Attestation commands and supporting packages | No attestation commands or packages |
| Codespaces | Codespace commands and supporting packages | No Codespaces commands or packages |
| Skills | Skill commands and supporting packages | No skill commands or packages |
| Tests | Unit, acceptance, and integration tests and fixtures | The test files and fixtures removed by the debloat change stay removed |
| Documentation | Upstream README and acceptance documentation | Those documents stay removed |
| Platforms | Linux, macOS, and Windows builds, installers, and runtime branches | Linux and macOS only. Windows is not supported: no Windows builds, installers, or runtime branches |

## When updating from upstream

Rebase on `cli/cli`'s `trunk`. Keep new upstream behavior outside the listed removals. Remove new code, dependencies, commands, and docs that restore a listed feature. Keep the fork buildable after each rebase.

Build with `make bin/gh`. Test with `make test`. [Install](docs/install.md), [release](docs/releasing.md), [other maintainer notes](docs/README.md).
