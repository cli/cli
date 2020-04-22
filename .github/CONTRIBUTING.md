## Contributing

[legal]: https://help.github.com/articles/github-terms-of-service/#6-contributions-under-repository-license
[license]: ../LICENSE
[code-of-conduct]: CODE-OF-CONDUCT.md

Hi! Thanks for your interest in contributing to the GitHub CLI!

We accept pull requests for bug fixes and features where we've discussed the approach in an issue and given the go-ahead for a community member to work on it. We'd also love to hear about ideas for new features as issues.

Please do:

* open an issue if things aren't working as expected
* open an issue to propose a significant change
* open a pull request to fix a bug
* open a pull request to fix documentation about a command
* open a pull request if a member of the GitHub CLI team has given the ok after discussion in an issue

Please avoid:

* adding installation instructions specifically for your OS/package manager

## Building the project

Prerequisites:
- Go 1.13

Build with: `make` or `go build -o bin/gh ./cmd/gh`

Run the new binary as: `./bin/gh`

Run tests with: `make test` or `go test ./...`

## Submitting a pull request

1. Create a new branch: `git checkout -b my-branch-name`
1. Make your change, add tests, and ensure tests pass
1. Submit a pull request: `gh pr create --web`

Contributions to this project are [released][legal] to the public under the [project's open source license][license].

Please note that this project adheres to a [Contributor Code of Conduct][code-of-conduct]. By participating in this project you agree to abide by its terms.

We generate manual pages from source on every release. You do not need to submit pull requests for documentation specifically; manual pages for commands will automatically get updated after your pull requests gets accepted.

## Resources

- [How to Contribute to Open Source](https://opensource.guide/how-to-contribute/)
- [Using Pull Requests](https://help.github.com/articles/about-pull-requests/)
- [GitHub Help](https://help.github.com)
