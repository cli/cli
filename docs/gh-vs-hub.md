# GitHub CLI & `hub`

[GitHub CLI](https://cli.github.com/) (`gh`) was [announced in early 2020](https://github.blog/2020-02-12-supercharge-your-command-line-experience-github-cli-is-now-in-beta/) and provides a more seamless way to interact with your GitHub repositories from the command line. We also know that many people are interested in the very similar [`hub`](https://hub.github.com/) project, so we wanted to clarify some potential points of confusion.

## Why didn’t you just build `gh` on top of `hub`?

We wrestled with the decision of whether to continue building onto `hub` and adopt it as an official GitHub project. In weighing different possibilities, we decided to start fresh without the constraints of 10 years of design decisions that `hub` has baked in and without the assumption that `hub` can be safely aliased to `git`. We also wanted to be more opinionated and focused on GitHub workflows, and doing this with `hub` had the risk of alienating many `hub` users who love the existing tool and expected it to work in the way they were used to.

## What’s next for `hub`?

The GitHub CLI team is focused solely on building out the new tool, `gh`. We aren’t shutting down `hub` or doing anything to change it. It’s an open source project and will continue to exist as long as it’s maintained and keeps receiving contributions.

## What does it mean that GitHub CLI is official and `hub` is unofficial?

GitHub CLI is built and maintained by a team of people who work on the tool on behalf of GitHub. When there’s something wrong with it, people can reach out to GitHub support or create an issue in the issue tracker, where an employee at GitHub will respond. 

`hub` is a project whose maintainer also happens to be a GitHub employee. He chooses to maintain `hub` in his spare time, as many of our employees do with open source projects.

## Should I use `gh` or `hub`?

We have no interest in forcing anyone to use GitHub CLI instead of `hub`. We think people should use whatever set of tools makes them happiest and most productive working with GitHub. 

If you are set on using a tool that acts as a wrapper for Git itself, `hub` is likely a better choice than `gh`. `hub` currently covers a larger overall surface area of GitHub’s API v3, provides more scripting functionality, and is compatible with GitHub Enterprise (though these are all things that we intend to improve in GitHub CLI). 

If you want a tool that’s more opinionated and intended to help simplify your GitHub workflows from the command line, we hope you’ll use `gh`. And since `gh` is maintained by a team at GitHub, we intend to be responsive to people’s concerns and needs and improve the tool based on how people are using it over time.

GitHub CLI is not intended to be an exact replacement for `hub` and likely never will be, but our hope is that the vast majority of GitHub users who use the CLI will find more and more value in using `gh` as we continue to improve it.
