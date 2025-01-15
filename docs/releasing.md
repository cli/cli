# Releasing

To initiate a new production deployment:
```sh
script/release vX.Y.Z
```
See `script/release --help` for more information.

> **Note:**  
> Every production release will request an approval by the select few people before it can proceed.

What this does is:
- Builds Linux binaries on Ubuntu;
- Builds and signs Windows binaries on Windows;
- Builds, signs, and notarizes macOS binaries on macOS;
- Uploads all release artifacts to a new GitHub Release;
- A new git tag `vX.Y.Z` is created in the remote repository;
- The changelog is [generated from the list of merged pull requests](https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes);
- Updates cli.github.com with the contents of the new release;
- Updates the [`gh` Homebrew formula](https://github.com/williammartin/homebrew-core/blob/master/Formula/g/gh.rb) in the [`homebrew/homebrew-core` repo](https://github.com/search?q=repo%3AHomebrew%2Fhomebrew-core+%22gh%22+in%3Atitle&type=pullrequests).

To test out the build system while avoiding creating an actual release:
```sh
script/release --staging vX.Y.Z --branch patch-1 -p macos
```
The build artifacts will be available via `gh run download <RUN> -n macos`.

## General guidelines

* Features to be released should be reviewed and approved at least one day prior to the release.
* Feature releases should bump up the minor version number.
* Breaking releases should bump up the major version number. These should generally be rare.

## Test the build system locally

A local release can be created for testing without creating anything official on the release page.

1. Make sure GoReleaser is installed: `brew install goreleaser`
2. `script/release --local`
3. Find the built products under `dist/`.

## Cleaning up a bad release

Occasionally, it might be necessary to clean up a bad release and re-release.

1. Delete the release and associated tag
2. Re-release and monitor the workflow run logs
3. Open pull request updating [`gh` Homebrew formula](https://github.com/williammartin/homebrew-core/blob/master/Formula/g/gh.rb) with new SHA versions, linking the previous PR
4. Verify resulting Debian and RPM packages, Homebrew formula
