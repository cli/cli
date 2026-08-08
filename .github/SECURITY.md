GitHub takes the security of our software products and services seriously, including the open source code repositories managed through our GitHub organizations, such as [cli](https://github.com/cli).

If you believe you have found a security vulnerability in GitHub CLI, you can report it to us in one of two ways:

* Report it to this repository directly using [private vulnerability reporting][].
  * Include a description of your investigation of the GitHub CLI's codebase and why you believe an exploit is possible.
  * POCs and links to code are greatly encouraged.
  * Such reports are not eligible for a bounty reward.

* Submit the report through [HackerOne][] to be eligible for a bounty reward.

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

A dependency having a CVE does not mean `gh` has a vulnerability. We use [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) to determine whether vulnerable symbols are actually reachable from `gh`'s code. If you are reporting a dependency CVE, please include evidence that the issue is exploitable in `gh`: a call chain into the affected symbols or a proof of concept. Reports that only list a dependency version and CVE without demonstrating impact will be closed.

Thanks for helping make GitHub safe for everyone.

  [private vulnerability reporting]: https://github.com/cli/cli/security/advisories
  [HackerOne]: https://hackerone.com/github
