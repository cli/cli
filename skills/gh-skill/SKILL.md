---
name: gh-skill
description: Manage agent skills with gh skill. Use this skill to discover, preview, install, update, and publish Agent Skills so an agent can self-manage the skills available in its environment.
---

# Managing skills with `gh skill`

`gh skill` installs, previews, searches, updates, and publishes
[Agent Skills](https://agentskills.io). An agent can use it to keep its
own skill set in sync with one or more GitHub repositories.

The command is also aliased as `gh skills`. Prefer the canonical singular
`gh skill` in scripts and docs.

## Search

```bash
gh skill search <query>                                  # free-text search
gh skill search <query> --owner <org>                    # restrict to one owner
gh skill search <query> --limit 20 --page 2
gh skill search <query> --json skillName,repo,description
```

## Preview before installing

```bash
gh skill preview <owner>/<repo> <skill-name>
gh skill preview <owner>/<repo> <skill-name>@v1.2.0   # pin a version
```

## Install

```bash
gh skill install <owner>/<repo> <skill-name>
gh skill install <owner>/<repo> <skill-name>@v1.2.0
gh skill install <owner>/<repo> skills/<scope>/<skill-name>   # exact path, fastest
gh skill install ./local-skills-repo --from-local
```

`<owner>/<repo>` and `<skill-name>` are both required.

Useful flags:

- `--agent <id>` - target host (e.g. `github-copilot`, `claude-code`,
  `cursor`, `codex`, `gemini-cli`). Repeat for multiple. Default is
  `github-copilot` when non-interactive. You should know what agent you are,
  so set this appropriately to install for yourself.
- `--scope project|user` - `project` (default) writes inside the current
  git repo; `user` writes to the home directory and applies everywhere.
- `--pin <ref>` - pin to a tag, branch, or commit SHA. Mutually exclusive
  with `--from-local` and with inline `@version` syntax.
- `--allow-hidden-dirs` - also discover skills under dot-directories such
  as `.claude/skills/`. Don't use this unless you need to, it comes with risks.
- `--force` - overwrite an existing install.

## Update

```bash
gh skill update --all          # update every installed skill
gh skill update <skill>        # update one
gh skill update <skill> --force
gh skill update --unpin        # drop the pin and move to latest
```

## Publish

Publishing turns a repo into a discoverable skill source. Skills are
discovered with these conventions:

- `skills/<name>/SKILL.md`
- `skills/<scope>/<name>/SKILL.md`
- `<name>/SKILL.md` (root-level)
- `plugins/<scope>/skills/<name>/SKILL.md`

Each `SKILL.md` needs YAML frontmatter:

```yaml
---
name: my-skill                # must equal the directory name
description: One sentence...  # required, recommended <= 1024 chars
license: MIT                  # optional but recommended
---
```

### Validate, then publish

```bash
gh skill publish --dry-run                 # validate only, no release
gh skill publish --dry-run ./path/to/repo  # validate a specific dir
gh skill publish --fix                     # auto-strip install metadata
gh skill publish --tag v1.0.0              # non-interactive publish
gh skill publish                           # interactive publish flow
```

`--fix` and `--dry-run` are mutually exclusive. `--fix` only rewrites
install-injected `metadata.github-*` keys and does not publish; commit
the result and re-run `publish`.

The publish flow will:

1. Add the `agent-skills` topic to the repo (so search can find it).
2. Use `--tag` (or prompt for one in a TTY).
3. Auto-push any unpushed commits.
4. Create a GitHub release with auto-generated notes.

Always pass `--tag` so it doesn't fall through to the interactive flow.

## Self-management pattern for agents

A reasonable loop:

1. `gh skill search <topic> --json skillName,repo,namespace`
2. `gh skill preview <repo> <skill>` to inspect the `SKILL.md`.
3. `gh skill install <repo> <skill> --agent <host> --pin <ref>` for a
   reproducible install.
4. Periodically `gh skill update --all` to refresh.