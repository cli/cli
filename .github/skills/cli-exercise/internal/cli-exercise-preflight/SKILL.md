---
name: cli-exercise-preflight
description: Internal readiness and installation-consent flow for cli-exercise. Requires entry-skill context and does not independently authorize a recording or installation.
user-invocable: false
license: MIT
---

# Preflight

Use the package root containing the user-facing `cli-exercise/SKILL.md`.
This module is bundled with that skill and is loaded as a referenced resource.

## Check without changing the machine

If the caller supplies a ready receipt already checked for this task, read and
reuse it when its selected tools and probed formats match the case. Do not repeat
probes or propose installation just because the receipt was produced elsewhere
in the same task. Recheck if its current applicability is uncertain. A receipt
does not grant installation or test-resource authority.
Normal session delivery, even for one case, needs MP4 chapter intermediates;
confirm MP4 capability as well as any requested GIF output.

When a check is needed, first follow the offline-first Go build instructions in
[the prerequisite reference](../../references/prerequisites.md). Build the helper
once for this task, outside the skill and repository, then use that executable:

```sh
<helper> --skill-root <package-root> preflight check --output <workspace>/preflight.json
```

Keep Go toolchain auto-downloads and module downloads disabled on the first
build. If a locked dependency is missing from the selected cache, show the
packages, sources, destination and build command, then ask before enabling its
download. A compile error is not permission to try an online build.

Explicit existing Node module, executable, and font paths can be supplied using
the documented overrides. Never embed operator-specific paths in the skill.

Check usable capabilities: the Node/Tuistory API and native PTY,
ffmpeg/ffprobe and requested encoders, and actual Go font rasterization.
Also identify any case-specific shell, editor, or executable. A command on PATH
does not prove that its native bindings or required API work.

## Ask before installing

When a dependency is missing or incompatible:

1. Check for a suitable existing installation before proposing changes.
2. Use the pinned manifests and platform context to propose exact setup commands.
   Show versions, sources, destinations, and any system/global changes.
3. Obtain caller approval for those commands.
4. Execute only the approved setup, then repeat the capability checks.

Use an appropriate dependency location outside the skill and repository.
Do not globally update tools or change project dependency manifests by default.
If system installation or authentication is required, state that separately.
Never enter an OS password or turn a failed privileged command into success.

If approval is declined, unavailable, or the install/recheck fails, report
`blocked` with the precise remedy. Preserve diagnostics and any partial-install
facts. Do not silently change recorders, output formats, or supplied cases.

The helper only checks capabilities. Perform approved setup with the appropriate
package manager.

## Keep authorization separate

Dependency approval does not authorize GitHub writes, filesystem changes outside
the workspace, credential changes, purchases, or other effects. Obtain the
required case-specific authorization separately and record its actual source.

The resulting receipt identifies tools used by the runtime and renderer.
It is readiness evidence, not an OS sandbox or a permission bypass.
