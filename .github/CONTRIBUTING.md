## Contributing

Hi! Thanks for your interest in contributing to the GitHub CLI!

We accept pull requests for bug fixes and features where we've discussed the approach in an issue and given the go-ahead for a community member to work on it. We'd also love to hear about ideas for new features as issues.

Please do:

* Check issues to verify that a [bug][bug issues] or [feature request][feature request issues] issue does not already exist for the same problem or feature.
* Open an issue if things aren't working as expected.
* Open an issue to propose a significant change.
* Open an issue to propose a design for an issue labelled [`needs-design` and `help wanted`][needs design and help wanted], following the [proposing a design guidelines](#proposing-a-design) instructions below.
* Open a pull request to fix a bug.
* Open a pull request to fix documentation about a command.
* Open a pull request for any issue labelled [`help wanted`][hw] or [`good first issue`][gfi].

Please avoid:

* Opening pull requests for issues marked `needs-design`, `needs-investigation`, or `blocked`.
* Opening pull requests that haven't been approved for work in an issue
* Adding installation instructions specifically for your OS/package manager.
* Opening pull requests for any issue marked `core`. These issues require additional context from
  the core CLI team at GitHub and any external pull requests will not be accepted.

## Building the project

Prerequisites:
- Go 1.22+

Build with:
* Unix-like systems: `make`
* Windows: `go run script/build.go`

Run the new binary as:
* Unix-like systems: `bin/gh`
* Windows: `bin\gh`

Run tests with: `go test ./...`

See [project layout documentation](../docs/project-layout.md) for information on where to find specific source files.

## Submitting a pull request

1. Create a new branch: `git checkout -b my-branch-name`
1. Make your change, add tests, and ensure tests pass
1. Submit a pull request: `gh pr create --web`

Contributions to this project are [released][legal] to the public under the [project's open source license][license].

Please note that this project adheres to a [Contributor Code of Conduct][code-of-conduct]. By participating in this project you agree to abide by its terms.

We generate manual pages from source on every release. You do not need to submit pull requests for documentation specifically; manual pages for commands will automatically get updated after your pull requests gets accepted.

## Design guidelines

### Proposing a design

You may propose a design to solve an open bug or feature request issue that has both [the `needs-design` and `help-wanted` labels][needs design and help wanted].

To propose a design:

- Open a new issue using the [design proposal issue template](./ISSUE_TEMPLATE/submit-a-design-proposal.md).
- Include a link to the issue that the design is for.
- Describe the design you are proposing to resolve the issue, leveraging the [CLI Design System][].
- Mock up the design you are proposing using our [Google Docs Template][] or code blocks.
  - Mock ups should cleary illustrate the command(s) being run and the expected output(s).

### (core team only) Revewing a design

A member of the core team will [triage](../docs/triage.md) the design proposal. Once a member of the core team has reviewed the design, they may add the [`help wanted`][hw] label to the issue, so a PR can be opened to provide the implementation.

## Resources

- [How to Contribute to Open Source][]
- [Using Pull Requests][]
- [GitHub Help][]


[bug issues]: https://github.com/cli/cli/issues?q=is%3Aopen+is%3Aissue+label%3Abug
[needs design and help wanted]: https://github.com/cli/cli/issues?q=state%3Aclosed%20is%3Aissue%20label%3Aneeds-design%20label%3A%22help%20wanted%22
[feature request issues]: https://github.com/cli/cli/issues?q=is%3Aopen+is%3Aissue+label%3Aenhancement
[hw]: https://github.com/cli/cli/labels/help%20wanted
[gfi]: https://github.com/cli/cli/labels/good%20first%20issue
[legal]: https://docs.github.com/en/free-pro-team@latest/github/site-policy/github-terms-of-service#6-contributions-under-repository-license
[license]: ../LICENSE
[code-of-conduct]: ./CODE-OF-CONDUCT.md
[How to Contribute to Open Source]: https://opensource.guide/how-to-contribute/
[Using Pull Requests]: https://docs.github.com/en/free-pro-team@latest/github/collaborating-with-issues-and-pull-requests/about-pull-requests
[GitHub Help]: https://docs.github.com/
[CLI Design System]: https://primer.style/cli/
[Google Docs Template]: https://docs.google.com/document/d/1JIRErIUuJ6fTgabiFYfCH3x91pyHuytbfa0QLnTfXKM/edit#heading=h.or54sa47ylpg
