# Working with the GitHub CLI Team: Hubber Edition

POV: your team at GitHub is interested in shipping a new command in `gh`.

This document outlines the process the CLI team prefers for helping ensure success both for your new feature and the CLI project as a whole.

## Step 0: Create an extension

Even if you want to see your code merged into `gh`, you should start with [an extension](https://docs.github.com/en/github-cli/github-cli/creating-github-cli-extensions) written in Go and leveraging [go-gh](https://github.com/cli/go-gh). Though `gh` extensions can be written in any language, we treat Go as a first class experience and ship a library of helpers for extensions written in Go.

Creating an extension enables you to start prototyping immediately, without waiting for us, and gives us something tangible to review if you decide you'd like the work incorporated into `gh`. It also means that you can decide to simply release your work without waiting for us to merge it, which leaves you in charge of release scheduling moving forward.

If you know from this point that you're comfortable with your new feature being an extension, don't worry about the rest of this document. We don't dictate how people create and release `gh` extensions.

If you do want your feature merged into `gh`, read on.

## Step 1: UX review

No matter what state your code is in, open up an issue either in [the open source cli/cli repository](https://github.com/cli/cli) or, if you'd rather not make the new feature public yet, [the closed github/cli repository](https://github.com/github/cli).

Describe how your new command would be used. Include mock-up examples, including a mock-up of what usage information would be printed if a user ran your command with `--help`.

We take this step seriously because we believe in keeping `gh`'s interface consistent and intuitive.

## Step 2: Beta

Once we've signed off on the proposed UX on the issue opened in step 1, develop your extension to at least beta quality. It's up to you if you actually want to go through a beta release phase with real users or not.

## Step 3: Merge or no merge

With a beta in hand it's time to decide whether or not to mainline your extension into the `trunk` of `gh`. Some questions to consider:

- How complex is the support burden for your feature?

If this feature requires extensive or specialized support, you will either need to release it as an extension or work with the CLI team to get maintainer access to `cli/cli`. The CLI team is very small and cannot promise any kind of SLA for supporting your work. For example, the `gh cs` command is sufficiently specialized and complex that we have given the `codespaces` team write access to the repository to maintain their own pull request review process. We have not put it in an extension as Codespaces are a core GitHub product with widespread use among our users.

- What kind of release cadence do you want?

We do a `gh` release roughly every other week, but if the changeset for a given week is light we may skip one. We make no official promise as to our cadence, and while we do have an on-call rotation there is no guarantee that you'll be able to get emergency fixes out within hours. If this is troubling, consider keeping your work in an extension.

- What kind of audience are you trying to reach?

Is this new feature intended for all GitHub users or just a few? If it's as applicable to your average GitHub user or customer as something like Codespaces or Pull Requests, that's a strong indication it should be merged into `trunk`. If not, consider keeping it an extension.

If after all of this consideration you think your feature should be merged, please open an issue in [cli/cli](https://github.com/cli/cli) with a link to your extension's code. It will go into our triage queue and we'll confirm that merging into `trunk` is feasible and appropriate.

## Step 4

Once we've signed off, open up a pull request in [cli/cli](https://github.com/cli/cli) adding your command. Since we make use of `go-gh` within our code already, it shouldn't be too onerous to make your extension merge-able. Link to the issue you opened in step 3 so we have some context on the pull request.

Keep in mind that our expectation of non-trivial commands that end up merged into `cli/cli` is that your team will continue to maintain what they merged over time. We can help redirect issues your way as part of our first responder rotation, but are unable to take on the full support burden for your new command.

## Other considerations

- If you have a high need for secrecy until the point of release, let us know in [#cli on slack](https://github.slack.com/archives/CLLG3RMAR). We'll come up with a solution to work on merging your command in private.
- We are a highly asynchronous team due to wide timezone differences. The best way to get in touch with us is via issue and pull request comments to which we'll respond within 24 hours. You can ping us on Slack but that's generally not our preference.
- We are happy to pair with you on extension authoring! Just let us know if we can provide guidance and we can schedule synchronous time to work together with you.
