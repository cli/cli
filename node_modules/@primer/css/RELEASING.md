# Releasing a new version of Primer CSS 🎉

## Prepare the release (in `primer/css`)

1. Decide which [PRs](https://github.com/primer/css/pulls) should be part of the next release and if it will be a major, minor or patch `<version>`. You may also check the [release tracking project
](https://github.com/primer/css/projects/2#column-4482699) or ask your team members in Slack.

1. Create a new release branch from `master` and name it `release-<version>`.

1. Run [`npm version <version>`](https://docs.npmjs.com/cli/version) to update the `version` field in both `package.json` and `package-lock.json`.

1. Create a new PR for the `release-<version>` branch. Please use the following template for the PR description, linking to the relevant issues and/or pull requests for each change, removing irrelevant headings and checking off all of the boxes of the ship checklist:

    ```md
    # Primer CSS [Major|Minor|Patch] Release

    Version: 📦 **0.0.0**
    Approximate release date: 📆 DD/MM/YY

    ### :boom: Breaking Change
    - [ ] Description #

    ### :rocket: Enhancement
    - [ ] Description #

    ### :bug: Bug Fix
    - [ ] Description #

    ### :nail_care: Polish
    - [ ] Description #

    ### :memo: Documentation
    - [ ] Description #

    ### :house: Internal
    - [ ] Description #

    ----

    ### Ship checklist

    - [ ] Update the `version` field in `package.json`
    - [ ] Update `CHANGELOG.md`
    - [ ] Test the release candidate version with `github/github`
    - [ ] Merge this PR and [create a new release](https://github.com/primer/css/releases/new)
    - [ ] Update `github/github`

    For more details, see [RELEASING.md](https://github.com/primer/css/blob/master/RELEASING.md).

    /cc @primer/ds-core
    ```

1. Start merging existing PRs into the release branch. Note: You have to change the base branch from `master` to the `release-<version>` branch before merging.

1. Update `CHANGELOG.md`

1. Wait for your checks to pass, and take note of the version that [primer/publish] lists in your status checks.

    **ProTip:** The release candidate version will always be `<version>-rc.<sha>`, where `<version>` comes from the branch name and `<sha>` is the 7-character commit SHA.


## Test the release candidate (in `github/github`):

1. Create a new branch in the `github/github` repo, name it `primer-<version>`.

1. Update the Primer CSS version to the published release candidate with:

    ```sh
    bin/npm install @primer/css@<version>-rc.<sha>
    ```

    Then commit and push the changes to `package.json`, `package-lock.json`, `LICENSE` and `vendor/npm`.

1. If you need to make changes to github/github due to the Primer CSS release, do them in a branch and merge _that_ into your release branch after testing.

1. Add or re-request reviewers and fix any breaking tests.

1. Test on review-lab.


## Publish the release (in `primer/css`)

1. If the release PR got approved and you've done necessary testing, merge it.

    After tests run, the docs site will be deployed and `@primer/css` will be published with your changes to the `latest` dist-tag. You can check [npm](https://www.npmjs.com/package/@primer/css?activeTab=versions) to see if [primer/publish] has finished.

1. [Create a new release](https://github.com/primer/primer/releases/new) with tag `v<version>`.

1. Copy the changes from the [CHANGELOG] and paste them into the release notes.

1. Publish 🎉


## Update github.com (in `github/github`):

1. Install the latest published version in the same `primer-<version>` branch created earlier with:

    ```
    bin/npm install @primer/css@<version>
    ```

    Then commit and push the changes to `package.json`, `package-lock.json`, `LICENSE` and `vendor/npm`.

1. Fix any breaking tests.

1. Deploy! :rocket:


### Publish the release

1. [Create a new release](https://github.com/primer/css/releases/new) with tag `v<version>`.

2. Copy the changes from the [CHANGELOG] and paste them into the release notes.

3. Publish 🎉


[changelog]: ../CHANGELOG.md
[primer/publish]: https://github.com/primer/publish
