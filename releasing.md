# Releasing

## Test Locally

`go test ./...`

## Push new docs

build docs locally: `make site`

build and push docs to production: `make site-docs`

## Release locally for debugging

A local release can be created for testing without creating anything official on the release page.

1. `env GH_OAUTH_CLIENT_SECRET= GH_OAUTH_CLIENT_ID= goreleaser --skip-validate --skip-publish --rm-dist`
2. Check and test files in `dist/`

## Release to production

This can all be done from your local terminal.

1. `git tag 'vVERSION_NUMBER' # example git tag 'v0.0.1'`
2. `git push origin vVERSION_NUMBER`
3. Wait a few minutes for the build to run and CI to pass. Look at the [actions tab](https://github.com/cli/cli/actions) to check the progress.
4. Go to <https://github.com/cli/cli/releases> and look at the release
