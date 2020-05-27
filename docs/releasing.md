# Releasing

_First create a prerelease to verify the relase infrastructure_

1. `git tag v1.2.3-pre`
2. `git push origin v1.2.3-pre`
3. Wait several minutes for the build to run <https://github.com/cli/cli/actions>
4. Verify the prerelease succeeded and has the correct artifacts at <https://github.com/cli/cli/releases>

_Next create a the production release_

5. `git tag v1.2.3`
6. `git push origin v1.2.3`
7. Wait several minutes for the build to run <https://github.com/cli/cli/actions>
8. Check <https://github.com/cli/cli/releases>
9. Verify the marketing site was updated https://cli.github.com/
10. Delete the prerelease on GitHub <https://github.com/cli/cli/releases>

## Release locally for debugging

A local release can be created for testing without creating anything official on the release page.

1. `goreleaser --skip-validate --skip-publish --rm-dist`
2. Check and test files in `dist/`
