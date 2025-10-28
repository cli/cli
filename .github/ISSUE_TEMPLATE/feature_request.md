<!--
Feature request template for GitHub CLI repository.
This template contains guidance for contributors to describe feature requests clearly.
-->

---
name: "Feature request / Propuesta de característica"
about: "Suggest an improvement or new feature"
labels: "feature"
---

# Descripción breve (Describe the feature or problem you’d like to solve)

- Provide a clear, concise description of the feature or problem.
- Include who is affected and a short example of the current behavior if applicable.

Ejemplo (example):
"When running `gh repo clone --shallow`, large repos time out on slow connections. I'd like an option to retry automatically and resume partial clones."

---

# Solución propuesta (Proposed solution)

- Describe the proposed change or feature in concrete terms.
- Explain how it will benefit the CLI and its users.
- If possible, sketch the UX (CLI flags, subcommands, defaults) and any API/behavior changes.

Ejemplo (example):
"Add a `--retry` and `--resume` flag to `gh repo clone`. `--retry` will attempt N times on network errors; `--resume` uses partial packs to continue cloning. This helps users on flaky networks and reduces wasted bandwidth."

---

# Contexto adicional (Additional context)

- Any screenshots, logs, or steps to reproduce.
- Related issues, PRs, RFCs or external resources.
- Backwards compatibility or migration notes.

Ejemplo (example):
- Link to related issue: #1234
- Reproduction steps: `gh repo clone big/repo --shallow` on < 2 Mbps link

---

# Criterios de aceptación (Acceptance criteria)

- List measurable, testable conditions that demonstrate the feature works.

Ejemplo:
- `gh repo clone` with `--resume` continues after network disconnect and finishes without restarting from zero.
- New flags have help text and unit tests cover edge cases.

---

# Alternativas consideradas (Alternatives considered)

- Briefly list other solutions that were considered and why the proposed approach was chosen.

---

# Impacto (Impact & risk)

- Does this change affect existing commands or flags? Breaking changes?
- Estimate scope: low/medium/high and possible performance/security concerns.

---

# Implementation notes (optional)

- Files or packages likely to change.
- Rough implementation sketch or pseudo-commands.

---

<!-- Spanish quick-guide for contributors -->
## Guía rápida en español
- Describa el problema o la mejora en pocas líneas (qué, quién, cuándo).
- Proponga una solución concreta: qué comandos/flags/UX cambiarían.
- Añada pasos para reproducir, logs o enlaces a issues relacionados.

Gracias por tu propuesta — un mantenedor revisará y podrá pedir más detalles si hace falta.
