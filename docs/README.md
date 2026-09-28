# Developing GitHub CLI

Shared guidance for anyone developing or reviewing `gh`. For installation and
usage, see the [GitHub CLI manual](https://cli.github.com/manual).

Before starting an external contribution, read
[CONTRIBUTING](../.github/CONTRIBUTING.md) for eligibility and scope. Report
security concerns through the [private disclosure process](../.github/SECURITY.md),
not public issues or pull requests.

## Find the guide for your task

| Task | Guide |
| --- | --- |
| Find the relevant source package | [Project layout](project-layout.md) |
| Set up and build locally | [Building the project](../.github/CONTRIBUTING.md#building-the-project) and [Go toolchain](../go.mod) |
| Design command syntax and terminal interactions | [Command-line syntax](command-line-syntax.md) and [Primer](primer/README.md) |
| Implement or review commands, flags, help, and output | [Command development](command-development.md) |
| Write tests and validate changes | [Testing](testing.md) |
| Work with API clients, host selection, and GHES capabilities | [API and hosts](api-and-hosts.md) |
| Change API gateway routing | [`api_host`](api-host.md) and its [test harness](api-host-test-harness.md) |
| Run or change live acceptance tests | [Acceptance README](../acceptance/README.md); running these requires explicitly authorized resources and credentials |
| Prepare a pull request | [PR template](../.github/PULL_REQUEST_TEMPLATE.md) |

Acceptance tests create and modify real GitHub resources. They are not part of
the ordinary unit-test loop; read their setup and cleanup requirements before
running them.
