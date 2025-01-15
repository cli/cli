# Guide to working with Codespaces using the CLI

For more information on Codespaces, see [Codespaces section in GitHub Docs](https://docs.github.com/en/codespaces).

## Access to other repositories

The codespace creation process will prompt you to review and authorize additional permissions defined in
`devcontainer.json` at creation time:

```json
{
  "customizations": {
    "codespaces": {
      "repositories": {
        "my_org/my_repo": {
          "permissions": {
            "issues": "write"
          }
        }
      }
    }
  }
}
```

However, any changes to `codespaces` customizations will not be re-evaluated for an existing
codespace.  This requires you to create a new codespace in order to authorize the new
permissions using `gh codespace create`.

For more information, see ["Repository access"](https://docs.github.com/en/codespaces/managing-your-codespaces/managing-repository-access-for-your-codespaces).

If additional access is needed for an existing codespace or access to a repository outside of
your user or organization account, the use of a fine-grained personal access token as an
environment variable or Codespaces secret might be considered.

For more information, see ["Authenticating to repositories"](https://docs.github.com/en/codespaces/troubleshooting/troubleshooting-authentication-to-a-repository).
