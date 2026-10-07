# skillscan: workspace scanner and installation-plan generator

## Purpose and boundaries

skillscan detects workspace technologies, resolves source-qualified skill recommendations, and produces deterministic plans for the upstream `skills` CLI. **Plan only—nothing installed.** Users review and run printed `npx skills` commands themselves.

The full-screen application, settings editor, banners/themes, simulated loading, Narsil integration, installer, status/update tracking, and speculative remote validation are removed. There are no compatibility aliases for removed commands or flags. This is an intentional breaking CLI simplification, not a scanner or provider redesign.

Discovery can make HTTP requests and write its existing user cache. Planning never starts a subprocess, looks for Node/npx, reads a legacy lockfile, or writes installed skills, tracking state, or configuration. Existing `.skillscan/lock.json` files and installed directories remain untouched. Applying plans and resolving conflicts with existing installations belong to the upstream tool.

## Command contract

| Command | Output |
| --- | --- |
| `skillscan` | Help and concise examples; no application shell |
| `skillscan scan [path]` on a terminal | Inline summary/skill selection, then a human-readable plan |
| `skillscan scan [path]` without a terminal | Deterministic report table; no prompts |
| `skillscan scan --json` | Existing versioned scan envelope, with advisory installation information |
| `skillscan scan --dry-run` | Structured plan for recommended local skills; no prompts |
| `skillscan plan <source> --skill <name>` | Validated human-readable plan and shell-quoted commands |
| `skillscan plan <source> --skill <name> --json` | Existing structured plan shape |
| `skillscan agent [path]` | Advisory agent assessment document, markdown or JSON |
| `skillscan backends list/check` | Search capabilities and provider health/probes |
| `skillscan version` / `completion <shell>` | Version and Cobra shell-completion scripts |

`--json` and `--dry-run` conflict on scan. `plan` requires at least one `--skill`; names and agents can be repeated or comma-separated. `--agent` and `--global` describe the generated plan, not permission to write anything. `--catalog`, `--max-depth`, `--online`, and verbose evidence remain relevant to scanning. Inline prompts run outside the alternate screen and write chrome/diagnostics to stderr; final plans and reports go to stdout. Machine-output modes bypass prompts.

Removed commands: `tui`, `install`, `status`, `update`. Removed flags: `--tui`, `--interactive`, `--yes`, `--allow-unvalidated`, `--narsil`, `--narsil-path`, `--skip-welcome`, `--debug`, `--log-level`. Cobra reports usage errors for them. The printed upstream argv still contains `--yes`; it is not a skillscan flag.

### Examples

```sh
# Inspect a project without discovery requests
skillscan scan . --online=false

# Deterministic machine assessment and recommended-local plan
skillscan scan . --json
skillscan scan . --dry-run --agent codex

# Inspect a specific skill reference, without detection or execution
skillscan plan vercel-labs/agent-skills --skill vercel-react-best-practices
skillscan plan vercel-labs/agent-skills --skill vercel-react-best-practices --agent codex --global --json

# Advisory brief for an agent
skillscan agent . --online=false
```

Human plans include workspace, scope, agents, selected skills, the read-only notice, and correctly quoted POSIX-shell commands. Review skills for trustworthiness before running any printed command. Structural validity does not prove that a skill is safe or supported by the upstream tool.

## Architecture

- `cmd/`: Cobra CLI, supported flags/default precedence, renderer selection.
- `config/`: explicit defaults and read-only standard-library JSON loading.
- `internal/walk/`: the only filesystem manifest source; exclusions, ignore rules, bounded depth, workspace members, and cancellation are retained.
- `internal/detect/`: existing layered technology detectors and signal merging.
- `internal/rules/`: declarative detection, technology, skill, and combo catalogs.
- `internal/resolve/`: recommendations, conflicts, and deterministic skill ordering.
- `internal/backend/`: all four search adapters, registry, queries, cache and probes.
- `internal/app/`: scan orchestration and pure `BuildPlan` selection integration.
- `internal/plan/`: source/name/agent validation, canonicalization, deduplication, destination-collision checks, deterministic adjacent-source batching, and argv construction.
- `internal/prompt/`: bounded Huh inline selection with built-in appearance.
- `internal/report/`: deterministic table, scan JSON, shared plan text and agent exports.
- `internal/textsafe/`: sanitization for repository-derived terminal text.

Detection/resolution remain local and unchanged. Online discovery enriches unresolved observations using focused queries and existing reciprocal-rank fusion. Keep `official-skills`, `skyll`, `skillsmp`, and `decimalai`, their endpoints, ranking behavior, limits, probes, and cache TTLs. All backend capabilities are search-only; there is no trust verdict or validation dispatch. `SKILLSMP_API_KEY` and `DECIMAL_API_KEY` remain supported.

## Selection and planning invariants

Recommended local skills are the only default selection. Possible and external suggestions require explicit inclusion. External search results retain their provenance, score, URL and stale-cache indication; backend ranking is not local confidence or trust.

Selection supports filtering and terminal-aware scrolling. Escape/Ctrl-C cancels the entire flow and prints no plan; it never submits a partially selected set. Zero selected items succeeds without a plan. An empty recommended set in `scan --dry-run` yields an empty structured plan.

`BuildPlan` with a nil selection picks recommended locals only; explicit selections must come from the scan result. Planning validates sources, skills and supported agents before producing commands. Reject control characters, option-like names, oversized/invalid references, and within-plan destination collisions. Canonical GitHub sources deduplicate consistently. Selections restore the existing deterministic scan-result order; adjacent same-source references batch together without globally regrouping across ordering edges. A source may appear in multiple batches. Legacy installation tracking does not participate in collision checks.

The structured shape is unchanged:

```json
{
  "root": "/absolute/workspace",
  "agents": ["codex"],
  "scope": "project",
  "batches": [
    {
      "source": "owner/repository",
      "skills": ["skill-name"],
      "argv": ["--yes", "skills", "add", "owner/repository", "--skill", "skill-name", "--agent", "codex", "--yes"]
    }
  ]
}
```

`argv` describes arguments to `npx`, not an executable invocation by skillscan. Global scope appends `--global`.

## Configuration and compatibility

Only settings with current consumers remain:

```json
{
  "scan": {"catalog": "", "maxDepth": 8, "online": true},
  "install": {"global": false}
}
```

Retain `install.global` for JSON compatibility although it now controls planning scope. Explicit flags override supported environment variables, which override config and then built-in defaults. `SKILLSCAN_CATALOG_URL` overrides configured catalog unless the CLI flag is explicit. Explicit false booleans are retained. The default is online discovery, depth 8, and project scope. Legacy configured depth 0 means 8; configured negative depths or values above 64 fail. Explicit CLI depths must be 1–64.

`--config` chooses an explicit document; missing or malformed explicit documents fail. A missing default document is allowed. Legacy removed keys are accepted and ignored; configuration is never rewritten. Debug/log environment variables no longer have consumers.

Locations stay fixed to the legacy `a-go-s` directory, independent of branding:

- Config: `$XDG_CONFIG_HOME/a-go-s/config.json`, or `~/.config/a-go-s/config.json`.
- Discovery cache: `$XDG_CACHE_HOME/a-go-s/registry`, or `~/.cache/a-go-s/registry`.

Scan JSON retains `schemaVersion: "1"`, `kind: "skillscan.scan"`, and existing fields including `install`, `installable`, and `installCommand`; these are advisory. Compact/verbose projections and empty-array conventions remain. Agent exports retain `templateVersion: "1"` and the `install`/`reject`/`unsure` assessment keys; an assessment never authorizes installation.

## Verification

Use the project's Go toolchain, without upgrading surviving dependencies or changing the module Go version:

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Coverage includes command/flag removals, config compatibility/precedence/depth, human and structured plans, shell quoting, canonical sources, deduplication/collisions/batching, recommended-only selection, external opt-in, no execution/tracking/config writes, malformed legacy lockfiles, prompt filtering/cancellation/empty/layout cases, and existing detector/resolver/walker/report/provider/cache/probe regressions.

Compare binary sizes using the same toolchain/build flags, writing verification binaries outside the repository. Leave existing binaries/build artifacts alone. Huh still uses Bubble Tea transitively; removing the full-screen app does not remove its inline-prompt runtime.
