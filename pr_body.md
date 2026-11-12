Closes #11247

This adds `--json` and `--jq` flags to `gh pr create`, matching the pattern used by other `gh pr` subcommands like `pr view`, `pr list`, and `pr status`.

## Summary of changes

- Added `Exporter cmdutil.Exporter` field to `CreateOptions`
- Registered `--json` and `--jq` flags via `cmdutil.AddJSONFlags` using `api.PullRequestFields`
- Added flag validation: `--json`/`--jq` are mutually exclusive with `--web` and `--dry-run`
- Modified `submitPR` to return `(*api.PullRequest, error)` instead of just `error`
- Added `finishSubmit` helper that re-fetches the created PR with the requested fields when an Exporter is configured, then writes JSON output
- Updated all three call sites of `submitPR` to use `finishSubmit`
- When `--json`/`--jq` is used, the plain URL output is suppressed so only JSON goes to stdout

## Example usage

```bash
gh pr create --title "Bug fix" --body "Fixes the bug" --json url,number,title
gh pr create --json url,number --jq '.number'
```

This allows scripting workflows like:
```bash
PR_NUMBER=$(gh pr create --title "..." --body "..." --json number --jq '.number')
```
