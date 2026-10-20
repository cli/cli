# GHCL-3: Lazy command routing

Status: accepted

## Summary

`NewCmdRoot` constructs every top-level command before Cobra dispatches the requested command. This RFC explores constructing only the selected command family for normal invocations.

## Motivation

The lazy reference change avoids generating reference text for ordinary commands. The root still constructs every command family. That work may add fixed CPU and allocation cost to every invocation, but the cost has not been measured. A router would reduce construction work. It would not remove package initialization for code linked into the executable.

## Design

Measure command construction first. If the cost matters, add a small dispatch layer that selects a top-level command family before constructing its command tree. Keep a full tree for root help, reference output, and shell completion. Preserve aliases, persistent flags, authentication checks, usage output, and command errors.

Use the existing eager tree as a fallback where routing cannot identify the same behavior, including configured aliases and unknown command paths. Do not add a second command registry unless it can stay in sync with the constructors and aliases already defined by the root.

## Alternatives considered

- Keep the eager tree and retain lazy reference generation. This avoids architecture changes but leaves all command constructors on the startup path.
- Remove package imports to reduce initialization work. This changes the linked program and requires separate evidence that package initialization limits startup.

## Open questions

- Which commands and alias forms can route without building the full tree?
- How can completion retain the complete command and flag list?
- What measured construction cost is enough to justify the router?
- Can Cobra preserve current help, error, and persistent flag behavior with a partial tree?

## Decision

No implementation is approved by this draft. Measure the current construction cost and resolve the open behavior questions before changing dispatch.
