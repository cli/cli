# Copy relase to another repo

**REMEMBER**
**If you want your changes to be run, make sure you transpile your typescript with the `npm run build` command**

This copies a relase from the repo the action is installed on to another repo.

Add these inputs to your workflow yml file.

```
  UPLOAD_OWNER_NAME: some-org-or-user
  UPLOAD_REPO_NAME: the-repo-name
```

Add this secret to your repo's secrets. It's a GitHub access token that has
access to the repo described by the UPLOAD_OWNER_NAME/UPLOAD_REPO_NAME inputs.

```
  UPLOAD_GITHUB_TOKEN: ${{secrets.UPLOAD_GITHUB_TOKEN}}
```
