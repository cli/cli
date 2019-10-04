# Contributing to Survey

🎉🎉 First off, thanks for the interest in contributing to `survey`! 🎉🎉

The following is a set of guidelines to follow when contributing to this package. These are not hard rules, please use common sense and feel free to propose changes to this document in a pull request.

## Table of Contents

1. [Code of Conduct](#code-of-conduct)
1. [Getting Help](#getting-help)
1. [Filing a Bug Report](#how-to-file-a-bug-report)
1. [Suggesting an API change](#suggesting-an-api-change)
1. [Submitting a Contribution](#submitting-a-contribution)
1. [Writing and Running Tests](#writing-and-running-tests)

## Code of Conduct

This project and its contibutors are expected to uphold the [Go Community Code of Conduct](https://golang.org/conduct). By participating, you are expected to follow these guidelines.

## Getting help

Feel free to [open up an issue](https://github.com/AlecAivazis/survey/v2/issues/new) on GitHub when asking a question so others will be able to find it. Please remember to tag the issue with the `Question` label so the maintainers can get to your question as soon as possible. If the question is urgent, feel free to reach out to `@AlecAivazis` directly in the gophers slack channel.

## How to file a bug report

Bugs are tracked using the Github Issue tracker. When filing a bug, please remember to label the issue as a `Bug` and answer/provide the following:

1. What operating system and terminal are you using?
1. An example that showcases the bug.
1. What did you expect to see?
1. What did you see instead?

## Suggesting an API change

If you have an idea, I'm more than happy to discuss it. Please open an issue and we can work through it. In order to maintain some sense of stability, additions to the top-level API are taken just as seriously as changes that break it. Adding stuff is much easier than removing it.

## Submitting a contribution

In order to maintain stability, most features get fully integrated in more than one PR. This allows for more opportunity to think through each API change without amassing large amounts of tech debt and API changes at once. If your feature can be broken into separate chunks, it will be able to be reviewed much quicker. For example, if the PR that implemented the `Validate` field was submitted in a PR separately from one that included `survey.Required`, it would be able to get merge without having to decide how many different `Validators` we want to provide as part of `survey`'s API.

When submitting a contribution,

- Provide a description of the feature or change
- Reference the ticket addressed by the PR if there is one
- Following community standards, add comments for all exported members so that all necessary information is available on godocs
- Remember to update the project README.md with changes to the high-level API
- Include both positive and negative unit tests (when applicable)
- Contributions with visual ramifications or interaction changes should be accompanied with the appropriate `go-expect` tests. For more information on writing these tests, see [Writing and Running Tests](#writing-and-running-tests)

## Writing and running tests

When submitting features, please add as many units tests as necessary to test both positive and negative cases.

Integration tests for survey uses [go-expect](https://github.com/Netflix/go-expect) to expect a match on stdout and respond on stdin. Since `os.Stdout` in a `go test` process is not a TTY, you need a way to interpret terminal / ANSI escape sequences for things like `CursorLocation`. The stdin/stdout handled by `go-expect` is also multiplexed to a [virtual terminal](https://github.com/hinshun/vt10x).

For example, you can extend the tests for Input by specifying the following test case:

```go
{
  "Test Input prompt interaction",       // Name of the test.
  &Input{                                // An implementation of the survey.Prompt interface.
    Message: "What is your name?",
  },
  func(c *expect.Console) {              // An expect procedure. You can expect strings / regexps and
    c.ExpectString("What is your name?") // write back strings / bytes to its psuedoterminal for survey.
    c.SendLine("Johnny Appleseed")
    c.ExpectEOF()                        // Nothing is read from the tty without an expect, and once an
                                         // expectation is met, no further bytes are read. End your
                                         // procedure with `c.ExpectEOF()` to read until survey finishes.
  },
  "Johnny Appleseed",                    // The expected result.
}
```

If you want to write your own `go-expect` test from scratch, you'll need to instantiate a virtual terminal,
multiplex it into an `*expect.Console`, and hook up its tty with survey's optional stdio. Please see `go-expect`
[documentation](https://godoc.org/github.com/Netflix/go-expect) for more detail.
