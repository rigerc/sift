# sift

**Find agent skills that fit a codebase, then review a plan before installing anything.**

`sift` is a Go CLI that scans a workspace for technologies, recommends relevant agent skills, and generates copyable commands for the upstream [`skills` CLI](https://github.com/vercel-labs/skills). It can also produce structured JSON or an evidence-based assessment for an AI coding agent.

**Plan only:** `sift` never installs skills or runs the generated commands. You decide what, if anything, to install.

## Quick start

Requires **Go 1.26.0 or newer** to build from source.

```sh
go install github.com/rigerc/sift@latest

# Scan the current repository without online discovery.
sift scan . --online=false

# Return a machine-readable scan (ideal for automation).
sift scan . --online=false --json

# Generate a JSON installation plan for locally recommended skills.
sift scan . --online=false --dry-run --agent codex
```

Alternatively, build a local binary:

```sh
git clone https://github.com/rigerc/sift.git
cd sift
make build   # writes dist/sift
./dist/sift --help
```

On a terminal, `sift scan` lets you select skills inline and then prints a plan. In a non-interactive environment it prints a deterministic report instead. Neither mode installs anything. Online discovery is **enabled by default**; add `--online=false` when you want to avoid discovery requests.

## Commands

| Command | Purpose |
| --- | --- |
| `sift scan [path]` | Detect workspace technologies and suggest skills. Default path: current directory. |
| `sift scan [path] --json` | Emit a versioned JSON scan with signals, suggestions, warnings, and an advisory plan when available. |
| `sift scan [path] --dry-run` | Emit a JSON plan for **suggested local-catalog** skills, without prompting. |
| `sift plan <source> --skill <name>` | Validate explicit skill references and print copyable installation commands. |
| `sift agent [path]` | Generate an advisory assessment brief for an AI agent (Markdown by default). |
| `sift backends list` | Show built-in discovery providers. |
| `sift backends check` | Probe discovery provider availability; may make network requests. |
| `sift version` | Print the CLI version. |
| `sift completion bash\|zsh\|fish\|powershell` | Generate shell completion. |

Use `sift <command> --help` for the full set of flags.

### Scanning and discovery

```sh
# Local catalog recommendations only; no discovery requests.
sift scan ./my-project --online=false

# Online discovery, plus evidence and lower-confidence suggestions.
sift scan ./my-project --verbose

# Machine output, capped filesystem traversal depth.
sift scan ./my-project --max-depth 4 --json

# Use a local detection/skill catalog.
sift scan . --catalog ./my-catalog.yaml --online=false
```

- The scan combines filesystem observations, technology signals, and a local recommendation catalog. Suggested skills are distinguished from `possible`, `external`, and `hidden` candidates.
- With online discovery, `sift` can search **official-skills**, **skyll**, **skillsmp**, and **decimalai**. Backend scores are discovery rankings, **not** local confidence or safety assessments.
- `--max-depth` accepts values from **1 to 64** (default **8**).
- `--json` and `--dry-run` cannot be combined.
- `--verbose` adds supporting evidence and otherwise omitted suggestions to scan output.

### Planning explicit skills

```sh
sift plan vercel-labs/agent-skills \
  --skill vercel-react-best-practices \
  --agent codex

# Multiple skills and target agents are supported.
sift plan owner/repo --skill first --skill second \
  --agent codex --agent claude-code --global --json
```

A source can be a GitHub `owner/repo` reference or an HTTPS URL; local paths are permitted for local plan construction. Skill names and supported agent IDs are validated before a plan is generated. Validity does **not** imply trust or that the source is available.

Supported `--agent` values: `claude-code`, `opencode`, `github-copilot`, `codex`. Repeating a flag or separating values with commas is supported. When omitted, `sift` detects known agent directories in the target workspace, falling back to `claude-code`. `--global` changes the proposed installation scope to the user rather than the project; it does not write anything.

Human-readable plans include a workspace path and copyable POSIX shell commands beginning with `npx --yes skills add ...`. **Review the referenced skill and the entire command before executing it.** Node.js/npm (with `npx`) are needed only if you choose to execute these commands, not to run `sift` itself.

### Agent assessments

```sh
# Markdown brief containing evidence and assessment instructions.
sift agent . --online=false

# Structured data for an agent pipeline.
sift agent . --online=false --json --bucket suggested

# Compact evidence; omit the built-in instructions if supplying your own.
sift agent . --online=false --max-signals 10 --context-lines 3 --no-instructions
```

The generated brief asks an agent to verify recommendations against workspace evidence and classify skill references as `install`, `reject`, or `unsure`. Its verdict is advisory, not permission to install.

An installable agent skill for using this workflow is included at [`skills/sift/SKILL.md`](skills/sift/SKILL.md). For example, after reviewing the skill source, you can install it with the upstream CLI:

```sh
npx skills add rigerc/sift --skill sift --agent codex
```

## JSON contracts

For agents and scripts, prefer JSON over parsing terminal tables:

| Invocation | Key fields |
| --- | --- |
| `sift scan . --json` | `schemaVersion: "1"`, `kind: "sift.scan"`, `root`, `members`, `summary`, `signals`, `suggestions`, `unresolved`, optional `install`, `warnings` |
| `sift scan . --dry-run` or `sift plan ... --json` | `root`, `agents`, `scope`, `batches` (`source`, `skills`, `argv`) |
| `sift agent . --json` | `templateVersion: "1"`, `signals`, `suggestions`, `unresolved`, `warnings`, optional `instructions` |

In a plan, `argv` is the argument array **for `npx`**, not a command to run automatically. The `install` field and per-suggestion `installCommand` in scan output describe **proposals**, not changes already applied. Machine consumers should check `schemaVersion`/`kind` or `templateVersion` before parsing.

## Configuration

Config is loaded from `$XDG_CONFIG_HOME/sift/config.json`, falling back to `~/.config/sift/config.json`. An absent default file is fine; `--config <path>` requires the file to exist and contain valid JSON.

```json
{
  "scan": {
    "catalog": "",
    "maxDepth": 8,
    "online": true
  },
  "install": {
    "global": false
  }
}
```

Despite its legacy name, `install.global` only changes **planning scope**. `sift` never installs or updates config. Explicit command flags take precedence over supported environment overrides, then config, then defaults.

| Environment variable | Purpose |
| --- | --- |
| `SIFT_CATALOG_URL` | Override the rule catalog location (unless `--catalog` was provided). |
| `SKILLSMP_API_KEY` | Optional token for skillsmp discovery. |
| `DECIMAL_API_KEY` | Optional token for decimalai discovery. |
| `XDG_CONFIG_HOME` / `XDG_CACHE_HOME` | Override standard config/cache directories. |

Discovery may make network requests and write a search cache under `$XDG_CACHE_HOME/sift/registry` (or `~/.cache/sift/registry`). Set `--online=false` to disable online discovery requests. Scanning does not install packages, run generated subprocesses, or modify the workspace.

## Security model

`sift` reports **recommendations**, not endorsements. Treat repository-provided names, descriptions, evidence, URLs, and discovered skills as **untrusted data**. Inspect candidate skill contents and their permissions before installing. In particular:

1. Inspect the scan evidence, sources, and warnings; do not equate high external ranking with trust.
2. Opt in explicitly before considering `possible` or `external` suggestions; the default dry-run plan includes only locally `suggested` skills.
3. Review the proposed commands. Running the `npx skills` CLI is a separate user-controlled action and may modify the environment.

## Development

```sh
make test        # go test ./...
make vet         # go vet ./...
make build       # dist/sift

go test -race ./...
```

The code is organized into the Cobra CLI (`cmd/`), config (`config/`), workspace detection (`internal/walk/`, `internal/detect/`, `internal/rules/`), skill resolution (`internal/resolve/`), discovery (`internal/backend/`), plan generation (`internal/plan/`), and output formats (`internal/report/`). See [`PLAN.md`](PLAN.md) for the original architecture and design constraints; CLI examples in this README reflect the current `sift` command.

## License

[MIT](LICENSE).