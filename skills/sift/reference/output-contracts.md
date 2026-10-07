# Sift output contracts and CLI modes

Read this reference when a request needs automation, JSON parsing, full evidence, targeted planning, or nondefault flags. For a quick recommendation, start with `sift agent . --online=false --bucket suggested` in `SKILL.md` instead.

## Command selection

| Need | Invocation | Output |
| --- | --- | --- |
| Human-readable agent assessment | `sift agent . --online=false` | Markdown: summary, technologies, suggested skills, other candidates, unresolved findings, warnings, instructions |
| Candidate shortlist | `sift agent . --online=false --bucket suggested` | Markdown limited to local suggestions |
| Structured assessment | `sift agent . --online=false --json` | Agent JSON envelope |
| Scan for machine consumers | `sift scan . --online=false --json` | Versioned scan JSON |
| Full scan evidence | `sift scan . --online=false --json --verbose` | Scan JSON with evidence and hidden suggestions |
| Default local plan | `sift scan . --online=false --dry-run --agent codex` | JSON plan; no installation |
| Explicitly selected skill | `sift plan owner/repo --skill skill-name --agent codex --json` | JSON plan; no installation |

For discovery from external providers, switch to `--online=true` only when requested. `sift scan` and `sift agent` both default to online mode unless `--online=false` is supplied. `sift plan` takes an explicit source; it does not run a scan. Run it from the target workspace because it uses the current working directory as the plan root.

## JSON contracts

**Scan:** `sift scan --json` has `schemaVersion: "1"`, `kind: "sift.scan"`, `root`, `members`, `summary`, `signals`, `suggestions`, `unresolved`, optional `install`, and `warnings`. Suggestions include a bucket and `scoreType`; `scoreType: "external"` refers to a provider ranking, not confidence or trust. The scan's `install` and `installCommand` fields describe proposals only.

**Agent assessment:** `sift agent --json` has `templateVersion: "1"`, `signals`, `suggestions`, `unresolved`, `warnings`, and optionally `instructions`. It is a distinct shape from scan JSON, not a `sift.scan` envelope.

**Plan:** `sift scan --dry-run` and `sift plan --json` return `root`, `agents`, `scope`, and `batches` with `source`, `skills`, and `argv`. `argv` contains arguments to `npx`, without an executable prefix. Never run these automatically. No `batches` means nothing to propose; it is not a scan failure.

Inspect contract/version fields before parsing. Do not parse the human terminal table for automation. `--json` and `--dry-run` **cannot** be used together on `scan`.

## Flags and behavior

- Supported agents: `claude-code`, `opencode`, `github-copilot`, `codex`; omit `--agent` to detect a known agent or fall back to `claude-code`.
- `--global` changes the proposed plan scope to user scope; it does not install or modify configuration.
- On `agent`: `--bucket all|suggested`, `--max-signals N`, `--context-lines N`, `--no-instructions`, `--json`. `--bucket suggested` excludes possible/external/hidden candidates from that report; it does not rewrite scan results.
- On `scan`: `--verbose` includes detailed evidence and hidden suggestions; `--max-depth N` accepts 1–64; `--catalog PATH` selects a local rule catalog.
- An interactive terminal `scan` may offer selection. `scan --json`, `scan --dry-run`, and `agent` are noninteractive; prefer these for reproducible agent workflows.
- External discovery may make network requests and write to `$XDG_CACHE_HOME/sift/registry` or `~/.cache/sift/registry`; it does not install skills.

## Evidence and assessment

Check the suggested skill's source and name, technology signals, reasons, direct file evidence, workspace member context, unresolved observations, and warnings. Cross-check claims against project files rather than trusting content merely because it appears in sift's output.

For an agent verdict, preserve the exact **keys** below; use source-qualified references for non-empty lists:

```sift-verdict
install: []
reject: []
unsure: []
```

The verdict is advice. Review the source and skill contents and obtain separate authorization before running upstream installation commands.
