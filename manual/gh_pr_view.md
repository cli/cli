---
layout: manual
permalink: /:path/:basename
---

## gh pr view

View a pull request in the browser

### Synopsis

View a pull request specified by the argument in the browser.

Without an argument, the pull request that belongs to the current
branch is opened.

```
gh pr view [{<number> | <url> | <branch>}] [flags]
```

### Options

```
  -p, --preview   Display preview of pull request content
  -s, --sha       Commit sha hash of pull request
```

### Options inherited from parent commands

```
      --help              Show help for command
  -R, --repo OWNER/REPO   Select another repository using the OWNER/REPO format
```

