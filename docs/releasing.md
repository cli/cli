# Releasing

## Release to production

This can all be done from your local terminal.

1. `git tag v1.2.3`
2. `git push origin v1.2.3`
3. Wait a few minutes for the build to run <https://github.com/cli/cli/actions>
4. Check <https://github.com/cli/cli/releases>

## Release locally for debugging

A local release can be created for testing without creating anything official on the release page.

1. `goreleaser --skip-validate --skip-publish --rm-dist`
2. Check and test files in `dist/`
