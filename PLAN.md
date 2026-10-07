# skillscan — Plan (rev 5, aligned to go-sense-3)

Scan a workspace for languages, frameworks, and patterns; suggest agent skills; install via `npx skills` CLI. External skill-discovery backends come later — this plan focuses on the **scanning engine** and the **matching/resolution logic** that turns detected signals into skill suggestions.

## Scaffold baseline and adaptation scope

This plan extends the application already in `go-sense-3/`. The current module and CLI are named `go-s`; **skillscan** remains the planned product/binary name used in future command examples below. Keep `module go-s` and its imports during implementation; building a binary named `skillscan` from `main.go` does not require a module rename. Update visible branding in M0, with config-path compatibility handled explicitly.

Verified against the scaffold on 2026-09-09:

| Area | Already implemented | Planned adaptation |
|---|---|---|
| Entry point | Root `main.go`; Cobra execution followed by optional TUI startup; logger/config/context initialization | Keep this entry point; share initialization and cancellation with feature subcommands |
| CLI | `cmd/root.go`, `version.go`, `completion.go`; `--config`, `--debug`, `--log-level`, `--skip-welcome`; root invocation opens the TUI | Extend `cmd/` with feature verbs as their milestones ship; preserve help/version/completion exits |
| UI shell | `internal/ui` root model, screen stack, resize/theme handling, header/banner, menu, help, modal, status/statusbar, spinner | Reuse the shell for workspace selection, scan results, evidence, and install progress |
| Screens | Home menu with Dashboard/Settings/Profile/About; welcome; working settings; simulated detail load | Replace demo destinations with product screens; retain welcome/settings and navigation conventions |
| Background work | `internal/task.Run`, `RunWithTimeout`, typed completion/error/progress messages | Adapt shared scan/install services into task commands; add operation cancellation and stale-result handling |
| Configuration | Root `config/`; koanf JSON defaults/load/save/validation; `cfg_*` settings schema; first-run/version helpers | Extend this package and its JSON schema; add scan/install/backend settings and runtime override handling |
| Logging | `internal/logger` uses the standard logger and `debug.log` | Reuse it; keep machine output clean and correct stale help claims about Zerolog |
| Dependencies/tooling | Go 1.26.0, pinned Charm v2/Cobra/koanf stack, tests, `.golangci.yml`, `.gitignore` | Add engine dependencies when used; extend existing tooling and add a Makefile |
| Domain features | No walk/detect/model/rules/resolve/install/report/backend packages or feature commands yet | Implement M1–M7; UI demos are not scanning or installation functionality |

Current configuration is JSON at `$XDG_CONFIG_HOME/a-go-s/config.json` (fallback `~/.config/a-go-s/config.json`), derived from the default `App.Name` of `A go-s`. The `--config` help currently says `go-s/config.json`, which differs from the implementation. Environment overrides are mentioned in scaffold comments/help but are not loaded. `main.loadConfig` applies only a true `--debug` override, silently falls back on load errors, and does not apply `--log-level`. These are M0 integration tasks, not existing capabilities.

Baseline validation: `go test ./...`, `go build ./...`, and `go vet ./...` pass. Existing coverage exercises config defaults/load/save/schema, root navigation/status, and the demo detail screen. It does not establish scanner, installer, backend, or CLI-routing coverage.

## Changes in rev 5

- Replace greenfield module/package creation with an incremental extension of root `main.go`, `cmd/`, `config/`, and `internal/ui`.
- Keep Bubble Tea as the application shell; embed selection/confirmation forms using the existing settings pattern.
- Put scan/resolve orchestration in a new UI-independent `internal/app` service shared by CLI commands and UI task adapters.
- Preserve JSON configuration and schema-driven settings; define missing runtime overrides, error handling, and branding migration explicitly.
- Record exact existing dependency pins separately from future additions and unverified upstream CLI compatibility assumptions.
- Rewrite M0/M4 and their gates around scaffold integration, command routing, task lifecycle, and UI regression coverage.

## Prior design changes (rev 4)

These describe the planned domain behavior retained from earlier revisions, not implemented scaffold features. Rev 5 supersedes earlier layout, configuration, and milestone assumptions.

- **[Blocker] Observation → technology → skill separation.** Detectors now emit raw `Observation` facts first. Local `DetectionRule`s map known observations to technology `Signal`s; `SkillRule`s then map technologies to installable skills. Unmapped, externally-eligible observations are retained for P3 discovery, so a package/framework that is absent from the embedded technology catalog can actually reach Stage 2.
- **[Blocker] Installer aligned to the real `skills` CLI contract.** A skill suggestion is now a `SkillRef{Source, Name}`, not a bare slug. Selected skills are source-qualified and converted into **stable source batches that preserve the resolved global install order**; each batch installs as `npx --yes skills@<pinned> add <source> --skill <names...> --agent <agents...> --yes`. A source may appear in more than one batch if cross-source ordering edges interleave it with another source.
- **[High] P2/P3 validation boundary corrected.** P2 performs structural `SkillRef`/agent validation only. `--allow-unvalidated` matters only when `--online` requests P3 authoritative validation and the result is `unknown`. Offline install does **not** manufacture an `unknown` verdict and does not require `--allow-unvalidated`.
- **[High] `--online` scope clarified.** It gates skillscan's discovery/validation/catalog HTTP backends only; it does not suppress network I/O performed by `npx skills add` for remote sources.
- **[High] Skillscan state no longer conflicts with the upstream CLI's lock files.** Skillscan writes `.skillscan/lock.json` for its own provenance/content hashes and never owns or rewrites `skills-lock.json` / `.skill-lock.json`. `ContentSHA256` is computed locally; optional upstream revision metadata is recorded only when discoverable.
- **[High] Detection schema can express its stated semantics.** Filename rules now distinguish exact vs glob matching; extension rules carry `MinCount`; content rules are structured. `nameIndex` remains exact-basename-only, with glob rules evaluated deterministically over sorted index keys.
- **[High] Go workspace parsing corrected.** `go.work`/`go.mod` use `golang.org/x/mod/modfile` (`ParseWork` / `Parse` or `ParseLax`); `go-toml/v2` is reserved for TOML formats such as `Cargo.toml` and `pyproject.toml`.
- **[Medium] Charm v2 module paths corrected.** TUI dependencies use `charm.land/huh/v2` and `charm.land/lipgloss/v2`.
- **[Medium] Combo confidence wording normalized.** Combos have no rule prior: `Suggestion.Confidence = min(triggering Signal.Confidence)`.
- **[Medium] Root/member semantics made explicit.** Root evidence and member evidence are scored independently; if both exist for one technology, merged detector confidence is the max of root confidence and member-weighted confidence. Root is not counted as a workspace member.
- **[Medium] Content-scan gating clarified.** Explicit configured content rules always run (so they can corroborate other evidence); generic fallback content reads run only for targets unresolved by Wave 1.
- **[Medium] Canonical upstream agent IDs used.** Agent targets passed to `skills` use upstream identifiers such as `claude-code`, `opencode`, and `github-copilot` rather than local aliases.
- **[Medium] External CLI compatibility must be pinned.** M5 records a tested `skills` CLI package/version and minimum Node version; subprocess tests assert the exact pinned argv. Updating that pin is an explicit compatibility task, not an implicit behavior change.

## Prior changes (retained)

- `internal/detect` layout is organized by pattern layer rather than language domain; per-language/framework knowledge lives as rules data.
- Workspace/monorepo detection runs immediately after the walk using the cached manifest, before detector waves, so member attribution requires no second disk walk.
- Detector output is deterministic: fixed wave/layer slots, fixed join order, deterministic de-dup/evidence ordering, final stable sort.
- `pyproject.toml` and `Cargo.toml` use real TOML parsing; `pnpm-workspace.yaml` uses YAML parsing; Go module/workspace files use `golang.org/x/mod/modfile`.
- Structural install validation is a P2 requirement; authoritative remote validation is P3 only.
- `ComboRule.Order` is modeled as ordering edges with deterministic topological sorting and explicit cycle errors.
- Backend integration uses an identity interface plus capability interfaces (`Searcher`, `Validator`, `Reporter`, `CatalogProvider`), a registry, and declarative `backends.yaml` configuration.
- Backend validation is three-valued (`valid` / `invalid` / `unknown`); stale validation never grants trust.
- A single backend cache is keyed by backend, capability, query hash, rules hash, and TTL; stale discovery may serve offline, stale validation may not.
- A concrete M0–M7 incremental build plan defines the Go stack, provider chain, acceptance gates, and golden tests.

## Projects combined

| Reference | Technique adopted |
|---|---|
| **autoskills** | Rich detector schema (deps, dep regexes, config/content/file/extension rules) + existence caching |
| **skillsense** | Declarative `combos.yaml` (`triggers/skills/conflicts`) mapping layer + external catalog URL override |
| **skillgrab** | Parallel detector ideas, provenance, and README/docs context hints for non-code skills |
| **askill** | Bounded parallel directory walk with SKIP_DIRS, per-agent install targets |
| **skillx** | Outcome reporting loop (later phase) |

## Architecture

Go application with an existing Bubble Tea shell and Cobra commands, plus a planned pinned `npx skills` installer. Keep domain packages independent of terminal rendering. `internal/detect` is organized by **pattern layer**, while rule data remains declarative. The tree below combines existing infrastructure with planned domain packages; it does not imply the new packages already exist.

```text
go-sense-3/                   existing module: go-s
  main.go                    existing executable entry point and lifecycle wiring
  cmd/                       existing root/version/completion; add feature command files
  config/                    existing JSON configuration, schema, save, paths, first-run helpers
  internal/
    logger/                  existing debug logger
    task/                    existing Bubble Tea async adapters
    ui/                      existing root model, navigation, message handlers
      screens/               existing home/welcome/settings/detail; add scan/results/install screens
      theme/                 existing palettes, styles, Huh theme integration
      header/, banner/, menu/, keys/, modal/, spinner/, status/, statusbar/
    app/                     NEW shared scan/resolve and install orchestration; no UI imports
    walk/                    NEW bounded walk; workspace partition; file indexes
    detect/                  NEW observation/matching layers
      manifest.go            layer 2 — manifest/package observations
      files.go               layer 3 — extension/filename/directory observations
      content.go             layer 4 — bounded content observations (Wave 2)
      context.go             layer 5 — README/docs/context observations
      match.go               Observation + DetectionRule -> per-member Signal
      merge.go               deterministic merge, confidence, member weighting
    model/                   NEW Observation, signals, rules, SkillRef, result/plan types
    rules/                   NEW rule loading and catalog merge
      data/                  embedded technology/detection/skill/combo YAML, owned by this package
    resolve/                 NEW local suggestions and unresolved observations for P3
    install/                 NEW ordered pinned npx spawn, validation, provenance
    report/                  NEW plain table/JSON rendering; interactive screens stay in ui/
      agent/                 NEW deterministic instruction brief + embedded template
    backend/                 NEW identity/capability interfaces and registry (P3)
      skillssh/, githubtrees/, semantic/, catalog/
```

Keep embedded YAML under `internal/rules/data/` so the owning package can embed it directly. The agent template similarly belongs inside `internal/report/agent/`.

### Application integration boundaries

- `main.go` owns process lifecycle and a signal-aware root context. Extract reusable configuration preparation into `config/`; pass runtime dependencies into command/UI wiring rather than duplicating startup in each verb. Currently feature subcommands would return before logger/config/context initialization, so M0 must correct that boundary.
- `cmd/` parses flags, calls shared `internal/app` operations, and renders headless results. Root invocation retains the home/welcome TUI. Interactive `scan` may explicitly launch the existing UI at its scan screen; `cmd.ShouldRunUI` must not cause a second launch afterward.
- `internal/app` coordinates `walk → detect → merge → resolve` and the shared install path using contexts and plain model values. It imports neither Cobra nor Bubble Tea, `internal/task`, or `internal/ui`. Domain packages receive typed options rather than depending on UI configuration/state.
- `internal/ui/screens` calls those services through `internal/task`; task functions return values and never mutate screen state from goroutines. Reuse `screens.Screen`, `NavigateMsg`, `BackMsg`, theme hooks, sizing, and status/modal messages.
- Give each scan/install operation a child context and operation ID. Cancel on user cancellation/navigation/quit, reject stale completions, and ensure service work observes cancellation. Existing navigation only changes the screen stack; `task.Run` cancellation does not forcibly stop its worker.
- Route task errors to the owning screen as well as the status bar. The current root handler consumes `task.ErrMsg`, so a loading screen cannot currently rely on receiving it to stop its spinner. Test this through the root model when adding real task screens.
- Use the existing embedded Huh form approach for selection/confirmation inside the one Bubble Tea program. `internal/report` produces table/JSON/brief output without starting a terminal program.

The pipeline has three distinct vocabularies:

1. **Observation** — raw bounded facts seen in the workspace (`pkg:npm:react`, `file:next.config.js`, `ext:.kt`, etc.).
2. **Technology signal** — a known technology ID produced when a local `DetectionRule` matches observations (`node:react`).
3. **Skill reference** — an installable skill identified by both upstream source and skill name (`vercel-labs/agent-skills` + `vercel-react-best-practices`).

This separation is the key Stage-2 invariant: a raw package/config observation can remain unresolved locally and still be sent to a discovery backend. Detection is therefore not limited to technologies that already have a local skill mapping.

## Data model

Three local confidence scales remain separate; external ranking remains a fourth non-comparable value.

```go
type ObservationKind string
const (
    ObsPackage ObservationKind = "package"
    ObsConfig  ObservationKind = "config"
    ObsExt     ObservationKind = "extension"
    ObsContent ObservationKind = "content"
    ObsContext ObservationKind = "context"
)

// Observation — raw workspace fact. It may or may not map to a known technology.
type Observation struct {
    Key      string          // "pkg:npm:react", "file:next.config.js", "ext:.kt"
    Kind     ObservationKind
    Domain   string          // ecosystem/domain hint: node, python, go, docs, ...
    Value    string          // raw package name / filename / extension / matched token
    Reason   string
    Evidence []string        // workspace-relative paths, sorted
    Layer    int
    Member   string          // workspace member root; "." means workspace root
}

// Signal — one known technology occurrence for one member/scope.
type Signal struct {
    Key      string          // Technology.ID, e.g. "node:react"
    Domain   string
    Reason   string
    Evidence []string
    Layer    int
    Member   string
}

// MergedSignal — layer-6 output and Stage-1 resolve input.
type MergedSignal struct {
    Key          string
    Domain       string
    Reasons      []string
    Evidence     []string
    Confidence   float64
    Members      []string    // distinct non-root member roots, sorted
    RootObserved bool
    Layers       []int       // agreeing detector layers, ascending
}

type MatchMode string
const (
    MatchExact MatchMode = "exact"
    MatchGlob  MatchMode = "glob"
)

type FileRule struct {
    Pattern string          // "angular.json" or "next.config.*"
    Mode    MatchMode       // exact uses nameIndex O(1); glob scans sorted index keys
}

type ExtensionRule struct {
    Extension string        // ".kt", ".exs"
    MinCount  int
}

type ContentRule struct {
    FilePattern string      // "Gemfile", "*.csproj", "build.gradle"
    Patterns    []string    // regexes, precompiled at rule load
    ReadLimit   int64       // 0 => default by file class
}

type DetectConfig struct {
    Packages        []string
    PackagePatterns []string
    ConfigFiles     []FileRule
    FileExtensions  []ExtensionRule
    Content         []ContentRule
    Manifests       []string
}

type Technology struct {
    ID, Name, Domain string
}

// DetectionRule answers "what technology does this workspace evidence indicate?"
type DetectionRule struct {
    TechnologyID string
    Detect       DetectConfig
}

// SkillRef is the install identity. Name alone is not globally unique.
type SkillRef struct {
    Source string // e.g. "vercel-labs/agent-skills", HTTPS/git source, or approved local source
    Name   string // skill name passed to `skills add --skill`
}

// SkillRule answers "which skills should this known technology suggest?"
type SkillRule struct {
    TechnologyID  string
    Skills        []SkillRef
    RuleConfidence float64 // default 1.0
}

type ComboRule struct {
    Triggers  []string    // Technology.ID values; all required
    Skills    []SkillRef
    Order     []SkillRef  // adjacent pairs create ordering edges a -> b
    Conflicts []string    // Technology.ID values; any present suppresses combo
}
```

`SkillRef` is keyed canonically by `(Source, Name)`. Duplicate suggestion merging, install ordering, validation results, and state entries all use that pair rather than a bare skill name. Because upstream installation paths are name-based, the install planner also enforces a destination-collision invariant: two different sources with the same skill `Name` cannot be installed to the same `(scope, agent)` target in one effective state; the user must choose one source (or use disjoint targets).

### Observation eligibility for external discovery

P3 does not indiscriminately send every raw fact to external search. Unmapped **package** observations are always eligible. Unmapped **config** observations are eligible when they are framework/tool-shaped rather than generic. Content observations are eligible only when they carry a stable identifier. Extension/context observations are included as query context but do not trigger external discovery by themselves. This bounds noise and query volume while preserving the "unknown framework" path.

## Scanning engine (phase 1 focus)

1. **Walk** (`internal/walk`) — one bounded parallel filesystem pass (worker count = `GOMAXPROCS`), respecting `SKIP_DIRS`, `.gitignore`, and maxDepth 8. It builds a cached file manifest plus `extCounts`, `nameIndex`, and `dirIndex`. Workspace roots (`pnpm-workspace.yaml`, `go.work`, `lerna.json`, `nx.json`, `package.json#workspaces`, `Cargo.toml#workspace`) are detected from that cache and the manifest is logically partitioned by member before detector waves. `go.work` is parsed with `modfile.ParseWork`; `Cargo.toml` uses TOML; `pnpm-workspace.yaml` uses YAML.

2. **Observe + match** (`internal/detect`) — layers run in two waves. Each layer writes observations to its own fixed slot; local matching is deterministic and synchronous at wave barriers.
   - **Wave 1** — layers 2, 3, and 5 run concurrently. These layers are independent/gating-free (not "content-free": manifests and context heuristics perform bounded reads).
   - **Wave-1 match barrier** — `match.go` maps Wave-1 observations through loaded `DetectionRule`s, producing known per-member `Signal`s plus an unresolved-observation set. This completed result defines Wave-2 fallback gating.
   - **Wave 2** — layer 4 runs bounded content scans. **Explicit `ContentRule`s always run** so content can corroborate manifest/config evidence. Generic fallback content reads run only for targets unresolved by layers 2–3.
   - **Wave-2 match** — new observations are mapped through the same deterministic matcher.
   - **Merge** — known signals are joined in fixed layer order `2 -> 3 -> 5 -> 4`, de-duped first by `(technology key, member)` and then globally, with deterministic evidence/reason union and confidence calculation. Unresolved externally-eligible observations are retained separately for P3.

3. **Resolve** (`internal/resolve`) — Stage 1 maps known technology signals through local `SkillRule`s and combos. Stage 2, when enabled, searches with unresolved observations plus known-signal context. See **Skill discovery & matching**.

4. **Install** (`internal/install`) — the install identity is `SkillRef{Source, Name}`.
   - Selected skills already have a global deterministic install order. The planner walks that order and coalesces **adjacent** SkillRefs with the same `Source` into one batch. It never globally regroups by source, because doing so could violate cross-source combo ordering edges.
   - Canonical invocation shape (for pinned integration version):
     `npx --yes skills@<pinned> add <source> --skill <name...> --agent <agent...> --yes [--global]`
   - Calls use `exec.CommandContext` with an argument array; no shell interpolation. The child `--yes` prevents a second confirmation after skillscan already obtained consent; the `npx --yes` prevents an npx package-install prompt.
   - **P2 structural validation** validates source syntax, skill names, and canonical upstream agent IDs before any spawn. Reject NUL/control characters, overlong values, option-like leading `-`, malformed source forms, and disallowed network-originated local paths. Spaces in skill names are not inherently rejected because argument-array spawning does not require shell quoting.
   - **P3 authoritative validation** runs only when `--online` is set and at least one `Validator` is enabled. Verdicts are `valid` / `invalid` / `unknown`: invalid blocks; unknown prompts interactively and under `--yes` requires `--allow-unvalidated`. With `--online` **off**, P3 validation is skipped entirely and structural validity is sufficient.
   - `--online` gates skillscan backend HTTP only. A remote `npx skills add` may itself use the network regardless of that flag.
   - Agent identifiers passed upstream use the upstream vocabulary, initially `claude-code`, `opencode`, and `github-copilot` for skillscan's auto-detected targets.
   - Before spawn, reject effective destination collisions: different `Source` values with the same skill `Name` may not target the same `(scope, agent)` installation namespace.
   - Skillscan writes `.skillscan/lock.json`, not the upstream CLI's lock filename. Entries contain `source`, `name`, scope/agents, `contentSha256`, `installedAt`, and optional upstream revision/ref metadata when discoverable. Only `contentSha256` is derived by hashing installed files. Hashing is path-stable: sort relative file paths, hash each normalized relative path plus file bytes, and hash the canonical skill directory rather than agent symlink metadata. The upstream `skills` CLI remains authoritative for its own update lock/state.
   - Runtime preflight checks `node`/`npx` and the pinned integration's minimum Node requirement before spawn, yielding a clear compatibility error rather than a child-process surprise.

5. **Output** — table (skill source + name + confidence + reason + evidence) or `--json`.

## Skill discovery & matching

`internal/resolve` consumes two inputs from detection: known `[]MergedSignal` and unresolved, externally-eligible `[]Observation`. Matching has a deterministic local stage and an optional external discovery stage.

### Stage 1 — deterministic rule matching (P1/P2, always runs, no network)

1. **Load rule sets.** Embedded `Technology`, `DetectionRule`, `SkillRule`, and `ComboRule` tables are loaded first. A local catalog override can merge over them by stable IDs. Detection rules and skill rules are deliberately separate: a technology may be locally detectable even if no local skill mapping exists.
2. **Known technology -> skill matching.** For every `MergedSignal`, look up `SkillRule` by `TechnologyID` (`signal.Key`). Each `SkillRef` receives the signal's provenance and local suggestion confidence `Signal.Confidence * RuleConfidence`. Complexity is O(number of signals + emitted skills) with map indexes.
3. **Combo matching.** A combo fires when every `Triggers` technology ID is present and none of `Conflicts` is present. Combo confidence is exactly `min(triggering Signal.Confidence)`; combos have no separate rule prior. Fired `Order` entries contribute global `SkillRef` ordering edges.
4. **Merge duplicate suggestions.** Merge by canonical `(SkillRef.Source, SkillRef.Name)`. Confidence takes the max across contributors; reasons/evidence are deterministic unions.
5. **Install order.** Merge all combo ordering edges and run Kahn topological sort with canonical SkillRef-key tiebreaks. A cycle is a rules error and names the cycle; no guessed ordering. Skills absent from the ordering graph sort after ordered skills by confidence descending, then canonical SkillRef key.
6. **Confidence model.** The scales remain distinct:

   | Scale | Carried by | Meaning | Source |
   |---|---|---|---|
   | **Detector confidence** | `MergedSignal.Confidence` | strength of workspace evidence for a known technology | detector layers + corroboration + scope weighting |
   | **Rule confidence** | `SkillRule.RuleConfidence` | trust in technology -> skill mapping | local rules prior; default 1.0 |
   | **Suggestion confidence** | local resolved suggestion | presentation/automation value | detector * rule; combo uses min trigger confidence |
   | **External score** | external suggestion | backend-native ranking only | backend search; never compared to local confidence |

   Detector base values: manifest/package 0.95, config-file 0.85, content 0.8, extension 0.6, context 0.4. Within one scope, corroboration is:
   `scope_confidence = min(1.0, highest_base + 0.1 * (agreeing_layers - 1))`

   Workspace/member handling:
   - non-root member support weight = `0.5 + 0.5 * (members_with_signal / total_members)`;
   - `member_base = max(per-member scope confidence across members exhibiting the signal)`;
   - member confidence = `member_base * support_weight`;
   - root evidence is scored independently and root is **not** part of `total_members`;
   - root-only signal => root confidence unchanged;
   - root + member evidence for the same technology => final detector confidence is `max(root_confidence, member_confidence)`.

   This avoids both penalizing root-wide tooling and falsely treating the workspace root as another package member.
7. **Bucketing.** Local suggestions: `>=0.7` suggested (pre-selected / eligible for `--yes`), `0.4–0.69` possible (shown, unchecked), `<0.4` hidden unless verbose. External suggestions never enter these buckets.

### Stage 2 — external fallback (P3, opt-in, requires backend network)

When `--online` is enabled, unresolved externally-eligible observations are sent to discovery backends. The query includes the raw unresolved observations plus known technology signals as context. This handles dependencies/configs that the local detection catalog does not recognize at all, not merely technologies lacking a local skill mapping.

Backend results must identify an installable `SkillRef{Source, Name}`. They are shown only in the **external suggestion** bucket, are never auto-selected, never installed by `--yes`, and retain their backend-native `ExternalScore` solely for display/ranking inside that bucket. The user must explicitly select an external result.

### Data flow summary

```
workspace files
    |
    v
Observation[]  ----------------------------------------+
    | known DetectionRule matches                      |
    v                                                   | unresolved + externally eligible
Signal[] per scope                                      |
    | deterministic merge                              |
    v                                                   |
MergedSignal[]                                          |
    |                                                   |
    +--> SkillRule + ComboRule --> local SkillRef[]     |
    |        |                                          |
    |        +--> local confidence buckets             |
    |                                                   |
    +---------------- P3 context -----------------------+--> Searcher backends
                                                            |
                                                            v
                                                   external SkillRef[]
                                                   (explicit opt-in only)

selected SkillRef[] -> global install order -> adjacent same-source batches -> pinned `npx skills add`
```

## Backend integration (universal discovery schema, P3)

`internal/resolve` never depends on a concrete API. Backends implement a small identity interface plus optional capability interfaces. Capability enablement is configured declaratively and checked against actual Go interface implementation at registry construction.

### Capability interfaces

```go
type Backend interface {
    Name() string
}

type Searcher interface {
    Search(ctx context.Context, q Query) ([]ExternalSuggestion, error)
}

type Verdict int
const (
    StatusInvalid Verdict = iota
    StatusValid
    StatusUnknown
)

type Validator interface {
    Validate(ctx context.Context, skills []SkillRef) (map[string]Validation, error)
}

type Validation struct {
    Status    Verdict
    Revision  string // display/provenance only; never substitutes for local content hash
    Downloads int
    Detail    string
}

type Reporter interface {
    Report(ctx context.Context, o Outcome) error
}

type CatalogProvider interface {
    Catalog(ctx context.Context) (RuleSet, error)
}

type Query struct {
    Unresolved []Observation
    Context    []MergedSignal
    Limit      int
}

type ExternalSuggestion struct {
    Skill         SkillRef
    Title         string
    SourceBackend string
    ExternalScore float64
    Reason        string
}

type Outcome struct {
    Skill    SkillRef
    Result   string // installed | rejected | success | failure
    Duration time.Duration
}
```

Validation maps use the canonical SkillRef key `(source, name)`. All capability methods are context-bounded.

### Capability authority

- Adapter maximum capabilities come from Go interface assertions, not a self-reported bitset.
- Config `capabilities:` is an enablement subset. Requesting a capability the adapter does not implement is a startup config error.
- `Caps` bit flags remain only as config/query vocabulary (`capSearch`, `capValidate`, `capReport`, `capCatalog`).

### Registry

```go
type Registry struct {
    backends map[string]Backend
    order    []string // config order = priority
}

func (r *Registry) ByCap(c Caps) []Backend
func (r *Registry) Get(name string) (Backend, error)
```

Construction instantiates each configured adapter, asserts implemented capability interfaces, checks the configured subset, and stores priority order. `ByCap` returns only enabled adapters implementing the requested interface.

Adapter packages:

```
internal/backend/
  backend.go       identity + capability interfaces, model, Caps vocabulary, Registry
  officialskills/  officialskills registry adapter          type: official-skills
  skyll/           Skyll search adapter                     type: skyll
  skillsmp/        SkillsMP search adapter                  type: skillsmp
  decimalai/       DecimalAI registry search adapter        type: decimalai
  httpx/           bounded JSON-over-HTTP client shared by HTTP adapters
  skillssh/        skills.sh discovery/validation adapter   type: skills.sh
  githubtrees/     GitHub source/tree validation adapter    type: github-trees
  semantic/        embedding-search adapter                 type: semantic
  catalog/         local/file catalog provider              type: catalog
```

### Config schema (`backends.yaml`)

```yaml
strategy: fanout            # fanout | first-hit
backends:
  - name: skillssh
    type: skills.sh
    url: https://api.skills.sh
    auth: env:SKILLSH_TOKEN
    capabilities: [search, validate]
    timeout: 3s
    cache_ttl: 24h

  - name: githubtrees
    type: github-trees
    auth: env:GITHUB_TOKEN
    capabilities: [validate]
    timeout: 5s

  - name: team-catalog
    type: catalog
    url: file://rules.yaml
    capabilities: [catalog, validate]

# select one path: --backends > SKILLSCAN_BACKENDS > config directory/backends.yaml;
# merge the selected document over built-in defaults by backend name.
```

### Where backends plug in

- **Stage-2 discovery.** Unresolved eligible observations go to enabled `Searcher`s. `fanout` runs them concurrently with per-backend timeouts; deterministic merge walks results afterward in registry priority order and de-dupes by canonical SkillRef key, so goroutine completion order never affects winners. `first-hit` runs sequentially until one backend returns at least one result.
- **Validation fan-in.** When `--online` is set before install, each selected SkillRef is offered to enabled `Validator`s in trust priority order. The first **authoritative** `valid` or `invalid` answer wins. Errors/timeouts/non-answers remain `unknown`; later validators may still answer. If no authoritative answer exists, final status is `unknown`.
- **Reporting.** `Reporter` is off by default and requires explicit configuration. It never turns on implicitly under `--yes`.
- **Catalog.** `CatalogProvider` supplies Technology/DetectionRule/SkillRule/ComboRule data **before detector execution**, because detection rules affect Observation->Signal mapping. Network-backed catalog providers are gated by `--online`; `file://` catalogs are local and remain usable offline. The resulting merged catalog is frozen for the scan and hashed into `rulesHash`.

### Cross-backend cache

One cache implementation is used for all cacheable backend capabilities, but the key schema is capability-aware and stored under skillscan state (not the upstream skills CLI lock location). Search/validation keys are `(backend, capability, requestHash, rulesHash)`. Catalog keys are `(backend, catalog, configHash)` because the catalog output itself contributes to the final `rulesHash`; using that output hash as an input key would be circular.

- Write only successful responses.
- Stale **discovery** responses may serve during an `--online` run when a live search backend is unavailable, clearly marked stale. With `--online` off, Stage 2 does not run and cached remote discovery is not consulted.
- Stale **validation** never grants `valid` or `invalid`; without a live authoritative answer, validation is `unknown`.
- Network catalog cache entries may serve stale only during an `--online` run when that configured catalog is unavailable; offline runs use embedded/local-file rules only. The catalog content hash contributes to the merged `rulesHash`.
- Cache serialization is deterministic; secrets/auth material are never part of the key or value.

### Fail-open and security invariants

- Discovery backend failure records a warning and never aborts the local scan/resolve path.
- Under `--online`, validation backend failure contributes no authoritative verdict; unresolved status becomes `unknown` and follows install policy. With `--online` off, remote validation is simply skipped.
- `--online` gates **skillscan backend network calls**, not the network behavior of a later `npx skills add` child process.
- Backend-returned SkillRefs are structurally validated exactly like local rule SkillRefs before display/install. Network backends may not return local filesystem sources; local sources are accepted only from explicit user/local catalog configuration.
- Network discovery prefers an installable GitHub `owner/repo` source and falls back to the provider's own skill detail URL when no repository is exposed. Detail URLs remain display/provenance candidates and may be rejected by the upstream install CLI; GitHub sources are always preferred so installs succeed whenever a provider exposes a repo.
- Auth values are `env:` references only, never logged, and URL query parameters are redacted from errors.
- HTTP adapters share bounded clients, response-size limits, and strict decoding.

### Adding a backend checklist

1. Implement one or more capability interfaces and a constructor/type string.
2. Register the constructor in `internal/backend/register.go`.
3. Add config docs and adapter tests using `httptest` or an in-memory stub.
4. Prove deterministic merge/timeout behavior in registry-level tests.
5. No changes to `walk/`, detector layer implementations, or install subprocess construction are required.

## CLI surface

Only the root TUI, `version`, and `completion` exist today. The following is the target surface, added incrementally. Until M0 branding, current commands run as `go run .`, `go run . version`, and `go run . completion bash` from `go-sense-3/`.

```text
skillscan                         # existing root TUI, adapted to the product
skillscan version
skillscan completion [bash|zsh|fish|powershell]
skillscan scan [--json] [--dry-run] [--yes] [--allow-unvalidated] [--verbose] [--online]
skillscan scan --catalog file://rules.yaml
skillscan scan --backends backends.yaml
skillscan backends list

skillscan install <source> --skill <name...> [--agent <agent...>] [--global]
                  [--yes] [--dry-run] [--online] [--allow-unvalidated]
skillscan status [--json]
skillscan update [name...] [--global|--project] [--yes]
skillscan agent [--json] [--bucket all|suggested] [--max-signals N] [--context-lines K] [--no-instructions] [--online]
```

Retain global `--config <config.json>`, `--debug`, `--log-level`, and `--skip-welcome`. `--verbose` controls evidence/report detail; it is separate from debug logging. No subcommand should fall through to the default root UI. `--json`, `--dry-run`, `--yes`, `agent`, and non-TTY output bypass welcome/settings/forms. A root invocation without a TTY returns a clear instruction to use `scan` or `agent`; it does not try to open the TUI.

`scan` scans the current working directory. On an interactive TTY with no scripting flag, it opens the scan/results flow in the existing UI. Non-TTY `scan` prints a report. `--json` alone is read-only; explicit `--yes` may request installation when M5 ships. Non-TTY install requires `--yes` unless `--dry-run` is used. Dry-run always exits without executing a child process.

`scan --yes` installs only local **suggested**-bucket results; external suggestions are never selected by `--yes`. Interactive scan presents local suggested/possible results and separately confirms external results.

`install` bypasses detection but uses the exact same internal install planner/spawn path as `scan`. It takes an upstream source plus one or more skill names because that is the actual `skills add` contract.

`--allow-unvalidated` is meaningful only when `--online` requested authoritative validation and one or more selected SkillRefs finish with `unknown`. Offline/P2-only installation requires structural validity but does not require this flag.

`update` delegates to the pinned upstream `skills update` command; skillscan does not reimplement upstream update resolution. After a successful update, skillscan refreshes content hashes/provenance for entries it tracks when their installed paths can be resolved.

`agent` is a fourth renderer over the same scan/resolve pipeline, alongside the interactive TUI, the plain table, and `--json`; see **Feature: `skillscan agent`** below.

### Machine-readable scan JSON (`scan --json`)

`scan --json` emits a versioned envelope (`internal/report.BuildScan`), not the raw `model.ScanResult`. Consumers branch on `schemaVersion` (currently `1`) and `kind` (`skillscan.scan`).

- **Compact by default:** `observations` is omitted, signals keep only `key`/`domain`/`confidence`, and each suggestion exposes one normalized `score` with an explicit `scoreType` (`confidence` for local, `external` for backend results). `--verbose` restores full signals and observations plus per-suggestion `reasons`/`evidence`/`technologies`/`members`.
- **No null collections:** `members`, `signals`, `suggestions`, `unresolved`, and `warnings` always serialize as arrays (`[]`).
- **Actionable installs:** each suggestion carries `sourceKind` (`github`/`url`/`local`/`unknown`), `installable`, and a copy-pasteable `installCommand`. A top-level `install` plan for the default **suggested**-bucket selection is attached when it builds; a build failure degrades to a structured warning instead of failing the scan.
- **Structured warnings:** `{"source": "...", "message": "..."}`, with backend fan-out failures tagged `discovery`.
- **Summary:** bucket/collection counts so an agent can branch before scanning arrays.
- `--dry-run` still emits the raw `install.Plan`.

## Feature: `skillscan agent` — machine-instruction output for AI agents

### Concept

A subcommand that runs the exact same scan/resolve pipeline as `scan`, but instead of rendering a human table or opening the Bubble Tea scan screen, it emits a **self-contained instruction document** for an AI agent (opencode, Claude, etc.). The agent consumes it, makes its own assessment of which skills are actually worth installing, and — if the invoking workflow allows — runs the install itself. It turns skillscan from an interactive tool into a **delegate-able decision brief**.

```
skillscan agent [--json] [--bucket all|suggested] [--max-signals N] [--context-lines K]
```

### Design

#### Pipeline

Identical one-code-path reuse of `walk → detect → merge → resolve` (same rule as the single `npx skills` spawn path — there is exactly one scan/resolve path in the binary; the root TUI, `scan` table/JSON output, and `agent` brief all consume the same shared service). Differences only at the render layer:

- No TUI, no multiselect, no `--yes` semantics — the agent is the decision-maker.
- Headless on both TTY and non-TTY; output is always deterministic and diff-stable (same `(domain, key)` sorting guarantees).

#### Output document (default: markdown, stdout)

The document has three parts, each derived from the planned pipeline output:

1. **Workspace assessment data** — the `MergedSignal` list with `Signal.Confidence`, agreeing `Layers`, `Reason`, `Evidence`, and `Members` provenance; plus unmatched signals (things detected but with no rule → the agent should reason about them rather than install blindly).
2. **Suggested skills** — the resolve output: `Suggestion.Confidence`, contributing `Reason`/`Evidence` (unioned), bucket (`suggested`/`possible`/`external`), and for externals the `Source` + `ExternalScore` explicitly labeled as backend ranking, not local confidence.
3. **Instruction block** — a stable, versioned prompt (shipped as an embedded template, `go:embed`, like the rules) telling the agent how to assess: verify claims against evidence, prefer corroboration, treat `possible`/`external` as opt-in, check for combo conflicts, and conclude with a structured verdict.

#### Verdict contract

The agent's assessment must end in a machine-parseable fenced block so orchestrating workflows can consume it without NLP:

````
```skillscan-verdict
install: [typescript-essentials, golang-concurrency]
reject: [marketing-seo]
unsure: [semantic-backend-stub]
```
````

`install` entries here are the **agent's assessment**, not an authorization — skillscan never reads this block back itself in P1 of the feature; a later `skillscan agent --apply-verdict` (out of scope for now) would be the only thing that does, gated like `--yes`.

#### Flags

| Flag | Behavior |
|---|---|
| `--json` | Same document as a JSON envelope (signals, suggestions, instruction text, template version) instead of markdown |
| `--bucket all\|suggested` | Trim the brief; default `all` including hidden-bucket items under `--verbose` semantics |
| `--max-signals N` | Cap signal list (highest confidence first) so the brief fits small context windows; default all |
| `--no-instructions` | Data only, omit the prompt block (for agents that already have their own workflow) |

#### Invariants inherited from the plan

- **Deterministic** — same sort guarantees; identical workspace → byte-identical brief (minus timestamps, which are omitted entirely).
- **Fail-open** — backend `unknown` verdicts appear as data with their status, never as errors; the instruction template tells the agent how to weigh `unknown`.
- **No secrets, no network** — `agent` runs in the offline Stage-1 mode by default; `--online` must be passed explicitly to include external suggestions, same gating as `scan`.
- **Security note for the template** — the instruction block must tell the agent that `Reason`/`Evidence` are scan-derived strings from workspace files and could contain prompt-injection content from a hostile repo; the agent should treat them as claims, not commands (mirrors autoskills' injection-scanning concern).

#### Where it fits

- No changes to `walk/`, `detect/`, `resolve/`, `install/` — one new `internal/report/agent` renderer (sibling to the M4 table/JSON renderer in `internal/report`) + an embedded instruction template + a cobra verb. Same "definition of done" as the backend checklist.
- Natural complement to P3 backends: `agent` is the output shape that makes delegated/orchestrated multi-agent flows (orca handoffs, CI bots) the primary consumers of skillscan rather than human TUI users.
- Golden tests: fixture workspace → byte-identical markdown brief + stable verdict-block schema; template version stamped in output so agents can detect instruction drift.

## Phases

1. **P1: Scanning engine** — one walk; workspace partition; observation-producing detectors in two waves; deterministic Observation->Signal matching; Signal merge/confidence/member weighting; local SkillRule/combo resolution; table/JSON output.
2. **P2: Install** — pinned upstream `skills` CLI integration; source-qualified SkillRefs; ordered adjacent-source batching; structural source/name/agent validation; destination-collision checks; agent targeting; `.skillscan/lock.json` provenance/content hashes; status/update delegation. P2 makes no **skillscan backend** network calls, though installing a remote source through `npx skills` naturally may use the network.
3. **P3: Backends** — capability interfaces/registry/config; external discovery over unresolved observations; source-qualified results; three-valued authoritative validation under `--online`; catalog/search/validation cache; optional reporting. `unknown` policy applies only when online validation was attempted but inconclusive.
4. **P3.5: Agent brief** — `skillscan agent`, a fourth renderer (`internal/report/agent`) over the same P1–P3 pipeline; emits a deterministic markdown/JSON instruction document plus a machine-parseable verdict contract for delegated/orchestrated agent workflows. No changes to `walk/`, `detect/`, `resolve/`, or `install/`. Depends on P3 only insofar as `--online` extends the brief with external suggestions; the offline P1/P2 brief ships independently.

## Scaffold & implementation plan

Incremental build order from the existing scaffold. M0 integrates the shell, M1–M4 deliver P1, M5 delivers P2, M6 delivers P3, and M7 delivers P3.5. Each milestone runs `go test ./...`, `go build ./...`, `go vet ./...`, and the repository's configured lint gate. Establish a compatible linter version/config in M0; baseline tests passing is not evidence that lint already passes. Add packages when they have work, rather than generating empty stubs for every future phase.

### Stack

Versions below are the current `go.mod` snapshot, not recommendations to upgrade.

| Library / integration | Scaffold status | Use in this plan |
|---|---|---|
| Go | `go 1.26.0`; module `go-s` | Keep toolchain/module baseline |
| `github.com/spf13/cobra` | direct `v1.10.2` | Extend existing command registration and flags |
| `charm.land/bubbletea/v2` | direct `v2.0.9` | Existing application loop and screens |
| `charm.land/bubbles/v2` | direct `v2.2.1` | Existing help/menu/spinner components |
| `charm.land/huh/v2` | direct `v2.0.3` | Embedded settings and planned selection forms |
| `charm.land/lipgloss/v2` | direct `v2.0.6` | Existing themes/styles and planned reports |
| `github.com/knadh/koanf/v2` | direct `v2.3.6` | Extend root `config/`; keep JSON persistence |
| koanf JSON parser / file / rawbytes providers | direct `v1.0.1` / `v1.2.1` / `v1.0.1` | Existing configuration pipeline |
| `github.com/stretchr/testify` | direct `v1.10.0` | Existing test conventions |
| figlet-go / go-colorful | already direct | Retain existing banner/theme support |
| `golang.org/x/sync` | indirect `v0.23.0` | Promote to direct when engine concurrency uses it |
| `gopkg.in/yaml.v3` | indirect `v3.0.1` | Promote to direct for rule/workspace/backend YAML |
| `github.com/pelletier/go-toml/v2` | absent; add and pin in M1 | Workspace/manifest TOML, not application config |
| `golang.org/x/mod` | absent; add and pin in M1 | Go module/workspace parsing |
| `github.com/go-git/go-git/v5` | absent; add and pin in M1 | Gitignore pattern parsing only |
| stdlib | available | JSON, regexp, paths, subprocesses, HTTP, hashing |
| upstream `skills` CLI | no integration present | Verify and pin installer compatibility in M5 |

The inherited installer proposal is `skills@1.5.25` with Node `>=22.20.0`. These are **proposed compatibility constants, not a tested scaffold capability**. Verify the package, minimum runtime, agent identifiers, and add/update argv before M5 is accepted; revise all examples and contract fixtures together if that baseline changes. Keep Go dependency additions incremental and pinned; no Charm/koanf replacement is required.

### M0 — Adapt the existing shell and runtime

- Keep root `main.go`, `cmd/`, `config/`, `internal/logger`, `internal/task`, and the UI component tree. Retain `module go-s`; do not run `go mod init` or introduce `cmd/skillscan/main.go`.
- Replace template branding/help/examples across Cobra, version/completion, `config.App`, welcome, and banner defaults. Keep existing config discovery compatible: continue reading/writing the legacy `a-go-s/config.json` path during this adaptation and decouple that path from display-name changes. `--config` always selects the exact user path. A move to a new default directory requires a separate explicit migration; changing `App.Name` alone must not strand settings.
- Move effective config preparation out of `main.loadConfig` into root `config/`; share it between UI and future feature commands. Preserve default/file loading and save/schema helpers; implement and test the target runtime overrides below.
- Establish signal-aware context and cleanup for both command and UI execution; keep help/version/completion free of TUI startup, first-run writes, and unnecessary initialization. Correct stale help claims about environment loading, embedded filesystems, and Zerolog.
- Define the UI-independent `internal/app` boundary and initial `internal/model` scan types when consumed. Add concrete command handlers with their milestones: `scan` in M4, `install/status/update` in M5, `backends` in M6, and `agent` in M7. Do not advertise successful placeholder feature commands.
- Extend the existing lint/ignore files as needed; add Makefile targets for build/test/lint/golden. Build the product binary from the root package (`go build -o dist/skillscan .`).
- **Gate:** existing tests remain green; help/version/completion exit without TUI; config path stays stable through display-name changes; precedence tests cover explicit false flags and log level; malformed explicit config returns a visible error. Root startup retains welcome/settings/theme/navigation behavior. Build/vet/lint pass with the established toolchain.

### M1 — Walk layer (`internal/walk`)

- Bounded parallel walk (`GOMAXPROCS`, maxDepth 8, SKIP_DIRS), gitignore parsing, fail-open warnings.
- Build manifest, `extCounts`, exact `nameIndex`, and `dirIndex` during the single walk.
- Layer-0 workspace detection from cached paths/content:
  - `go.work` via `modfile.ParseWork`;
  - `package.json#workspaces` via JSON;
  - `Cargo.toml#workspace` via TOML;
  - `pnpm-workspace.yaml` via YAML;
  - lerna/nx markers as appropriate.
- Partition cached manifest by member root with root scope represented separately from members.
- **Gate:** golden fixture trees assert manifest/index/member partition; malformed workspace files fail open with warnings; `go.work` multi-use fixtures exercise `ParseWork`.

### M2 — Observation + detector layers (`internal/detect`)

- `manifest.go`: parse all dependencies from flagged manifests and emit raw package observations whether or not they are known locally. JSON for package/composer manifests; TOML for pyproject/Cargo; `x/mod/modfile` for Go modules; bounded legacy line parsing only where the source format is line-oriented (`requirements.txt`, Gemfile subset).
- `files.go`: emit/configure filename, directory-prefix, and extension-count observations. Exact filename rules hit `nameIndex`; glob filename rules evaluate against sorted basename keys; extension rules enforce per-rule `MinCount`.
- `context.go`: bounded README/description/docs observations.
- Wave-1 matcher: Observation + DetectionRule -> per-member Signal; retain unresolved observations.
- `content.go`: explicit ContentRules always execute; generic fallback only on unresolved targets. Enforce read caps and binary sniff.
- Wave-2 matcher uses the same mapping code.
- **Gate:** parser table tests, unknown-package observation retention test, exact-vs-glob filename tests, extension threshold tests, content-read caps, and wave barrier test proving Wave 2 sees complete Wave-1 match results.

### M3 — Merge + Stage-1 resolve (`detect/merge.go`, `internal/resolve`, `internal/rules`)

- Fixed-order merge `2 -> 3 -> 5 -> 4`; per-`(technology, scope)` de-dup then global technology merge; deterministic reasons/evidence/layers.
- Confidence tests for within-scope corroboration, member support weighting, root-only behavior, and root+member max rule.
- Embedded YAML split into technology metadata, detection rules, skill rules, and combos; local catalog override merge by stable IDs.
- Local resolve produces source-qualified SkillRefs, merges by `(source,name)`, applies rule confidence, buckets local results.
- Combo triggers/conflicts, `min(trigger confidences)`, SkillRef ordering edges, deterministic topo sort and cycle errors.
- Preserve unresolved externally-eligible observations as an explicit resolver output for M6 rather than discarding them.
- **Gate:** golden JSON includes observations/signals/suggestions as appropriate; unknown framework package survives through `Unresolved`; duplicate same-name skills from different sources remain distinct; combo ordering/cycle tests pass.

### M4 — Shared scan service, reports, and existing TUI integration

- Wire M1–M3 through `internal/app` once. Add `cmd/scan.go` for headless table/JSON output and explicit interactive scan startup; root home navigation uses the same service.
- Adapt `screens/home.go` to offer Scan workspace, Settings, and About. Replace simulated detail loading with results/evidence screens using `screens.Screen`; add installed-skill/status navigation in M5 and backend navigation in M6 when their services exist.
- Run scanning via `internal/task`, with spinner/status progress, cancellation, and operation IDs. Fix root task-error forwarding so failures clear loading state; test navigation/quit and delayed results through the root update loop.
- Reuse existing theme/resize/help/modal components and embedded Huh settings conventions. Show local suggested results preselected and possible results unchecked; add a separate explicit external-result selection in M6.
- Render table columns for bucket, source, skill, confidence/external score, reason, and evidence. JSON and agent-facing output must not contain UI control sequences, logger output, or welcome prompts.
- M4 is scan/report/selection only. The dry-run install planner and executable `--yes`/install actions ship together in M5; do not make installation part of the P1 acceptance gate.
- **Gate:** fixture scan gives identical service data through CLI and UI; golden table/JSON output; root-level scan success/error/cancel tests; selection/form tests; navigation, resize, theme, settings persistence, and welcome regressions remain green. Non-TTY scan and `scan --json` complete without constructing a TUI.

### M5 — Install (P2, `internal/install`)

- Add `cmd/install.go`, `cmd/status.go`, and `cmd/update.go`; connect UI selection/confirmation/progress and Installed skills navigation to the same `internal/app` install operations.
- Verify the proposed upstream CLI baseline, then centralize the tested package/runtime constants in `internal/install`. Add Node/npx preflight only to install/update execution; scanning, dry-run planning, and normal TUI startup must work without Node.
- Enable `scan --yes` and `--dry-run` now. Dry-run renders the ordered source batches, skill names, agents, scope, and exact child argv without execution. Embedded confirmations use the existing modal/form flow; child output must not corrupt either the active TUI or JSON stdout.
- Build an install plan from the resolved global SkillRef order. Coalesce only adjacent entries sharing the same normalized source; never regroup across an ordering edge. A source can therefore yield multiple subprocess batches.
- Single subprocess-construction function emits:
  `npx --yes skills@1.5.25 add <source> --skill <names...> --agent <agents...> --yes [--global]`
  using `exec.CommandContext`; no shell strings.
- Structural validation before spawn:
  - source parser/allowlist, no control/NUL/leading-option ambiguity;
  - skill name non-empty/bounded/no control/path separator/leading-option ambiguity;
  - canonical agent IDs only;
  - network/catalog suggestions cannot inject local filesystem sources.
- Preflight `node` + `npx`; enforce the pinned integration's minimum Node version with a clear error.
- P2 has **no authoritative validator** and no `unknown` verdict. `--allow-unvalidated` has no effect unless M6/P3 online validation runs.
- Upstream network behavior is not controlled by `--online`; remote sources may fetch normally.
- Reject `(scope, agent, skill-name)` collisions when selected entries come from different sources.
- Write `.skillscan/lock.json` entries with source, name, agents, scope, content SHA-256, installedAt, and optional discovered upstream ref/revision. Hash the canonical installed skill directory using sorted normalized relative paths + file bytes; never hash agent symlink metadata. Never write/replace upstream `skills-lock.json` / `.skill-lock.json`.
- `status` reads skillscan state plus installed paths; `update` delegates to pinned `npx skills update` then refreshes tracked hashes when resolvable.
- **Gate:** CLI/UI selection produces the same install plan; dry-run golden and fake runner prove zero execution; install failure clears UI loading state; fake `node`/`npx` shims assert exact argv and version preflight; interleaved cross-source ordering test proves batching preserves global order; canonical `github-copilot` agent test; state round-trip/hash stability; same-name/different-source destination collision is rejected; no validator-related gate in M5.

### M6 — Backends (P3, `internal/backend` + adapters)

- Add `cmd/backends.go`, backend configuration loading in root `config/`, and backend status in the existing UI; keep credentials out of persisted settings/forms.
- Capability interfaces + registry + config subset validation.
- Adapters: catalog first, then skills.sh Searcher/Validator, GitHub Trees Validator, semantic-search contract.
- Stage-2 input is unresolved `Observation[]` plus known-signal context; result contract requires `SkillRef{Source,Name}`.
- Fanout deterministic merge by backend priority and SkillRef key; first-hit sequential behavior.
- Online validation fan-in over SkillRefs: first authoritative valid/invalid wins; failures/nonanswers remain unknown; `--yes` + unknown requires `--allow-unvalidated`.
- Single backend cache with stale-discovery-allowed / stale-validation-never-trusted semantics.
- Shared bounded HTTP client, response-size cap, strict decode, URL/auth redaction; `--online` gate applies to backend HTTP/catalog network calls only.
- **Gate:** unknown package -> external search golden; same skill name from different sources remains distinct; dead validator -> unknown and M6 install-policy test; `--online` off -> validator not called and install remains P2-only; fanout completion-order test proves deterministic priority winner.

### M7 — Agent brief (P3.5, `internal/report/agent`)

- New renderer package, sibling to the table/JSON code added in M4 in `internal/report`; consumes the same `[]MergedSignal` + resolve output as `scan`, adds nothing to `walk/`, `detect/`, `resolve/`, or `install/`.
- Embedded, versioned instruction template (`go:embed`, alongside the rules embed) covering: verify claims against evidence, prefer corroboration, treat `possible`/`external` as opt-in, flag combo conflicts, treat scan-derived `Reason`/`Evidence` strings as claims rather than commands (prompt-injection caution for hostile-repo content), and close with the `skillscan-verdict` fenced-block contract.
- `--json` emits the same document as a structured envelope (signals, suggestions, instruction text, template version) instead of markdown.
- `--bucket`, `--max-signals`, `--context-lines`, and `--no-instructions` trim/shape the brief; `--online` gates external suggestions exactly like `scan`.
- Add `cmd/agent.go`, wired as headless output on both TTY and non-TTY: no huh forms, no `--yes` semantics — the agent, not skillscan, is the decision-maker. `skillscan-verdict` output is not read back by skillscan in this milestone (`--apply-verdict` is explicitly out of scope, to be gated like `--yes` if built later).
- **Gate:** golden fixture workspace → byte-identical markdown brief across runs (timestamps omitted); `--json` envelope schema test; verdict fenced-block schema test; template-version stamp changes only on deliberate template edits; injection-caution wording present in template golden.

### Config loading (existing koanf, root `config/`)

Extend `config.Config`, `Load`, `LoadFromBytes`, `Save`, `DefaultConfig`, and the `cfg_*` schema rather than adding a competing `internal/config` package. Keep `config.json` as the application format; YAML is for rules/workspace metadata and the separate P3 backend document. TOML is for workspace manifests.

Current behavior is defaults → JSON file, followed by a true-only debug override in `main.go`. The **target runtime chain**, to implement and test in M0, is:

```text
cfg_default struct tags -> JSON config file -> runtime environment -> explicitly changed Cobra flags
  config path: --config, otherwise config.DefaultConfigPath()
  default:     $XDG_CONFIG_HOME/a-go-s/config.json, otherwise ~/.config/a-go-s/config.json
  env:         SKILLSCAN_ prefix; explicit typed mapping to supported config keys
  flags:       apply only flags marked changed, including explicit false values

P3 backends.yaml: separate loader in root config/;
  path precedence = --backends > SKILLSCAN_BACKENDS > config directory/backends.yaml
```

- Keep supported env names explicit (`SKILLSCAN_DEBUG`, `SKILLSCAN_LOG_LEVEL`, then catalog/backend/scan/install options as added). Do not claim a generic nested env provider already exists. Keep runtime overrides separate from persisted settings so saving a theme does not persist one-shot install consent, CLI choices, or environment secrets.
- Add scalar Scan/Install settings groups with `json`, `koanf`, `cfg_default`, label, and description tags. Existing form accessors handle string/bool/int fields; slices, maps, durations, or deeper nesting need a deliberate editor/validation extension, or exclusion of the entire group from generic settings. Backend lists stay in their separate document.
- Preserve defaults-first loading for older JSON files. Use `ConfigVersion`/`NeedsUpgrade` for breaking schema migrations; the helper detects version differences but does not perform a migration today.
- Missing default config is a valid first run. Missing explicitly requested config and malformed known values return visible errors in feature commands; TUI startup presents an actionable error before any save can overwrite the file. Add unknown-key warnings and strict known-value decoding as implementation work; current `Validate` only checks log level.
- Keep atomic save via the existing save path and retain welcome completion/settings save behavior. Headless commands must not trigger first-run configuration writes.
- Resolve backend durations and `auth: env:VAR` only when constructing runtime backend options; never persist/log resolved auth values. A backend path selects one document; it is not a chain of files to merge.
- `SKILLSCAN_CATALOG_URL` remains the planned local/file or configured catalog shortcut. Network catalog access is subject to `--online`.
- Keep the upstream CLI pin in tested installer compatibility constants. Ordinary environment/config values cannot silently float it to `latest`; any future explicit compatibility override must appear in verbose/dry-run output.

### Go-specific standards

- Concurrency: errgroup + bounded pools, fixed-order join for deterministic merge (golang-concurrency)
- Errors: wrap with `%w` at boundaries (golang-error-handling)
- Tests: table-driven, golden files for detector *and* resolve output (golang-testing)
- No secrets logging; bounded file reads; subprocess calls always use argument arrays, never interpolated shell strings (golang-security)

## Scanning patterns

Seven semantic layers remain numbered 0–6. Execution order is: Layer 1 walk -> Layer 0 workspace partition -> Wave 1 (2,3,5) -> Wave-1 match barrier -> Wave 2 (4) -> merge (6). Layer numbers describe role, not chronological order.

### Layer 0 — Workspace/monorepo detection (`internal/walk`)

- Runs after the file-manifest pass against the existing cache.
- Parsers: `pnpm-workspace.yaml` YAML; `go.work` `modfile.ParseWork`; `package.json#workspaces` JSON; `Cargo.toml#workspace` TOML; lerna/nx marker/config parsing as needed.
- Partitions the cached manifest by workspace member root without a second filesystem walk.
- Root scope `.` is tracked separately from member roots. Root observations are not counted in the workspace member denominator.
- If no workspace marker is found, the repository is treated as a single root scope and member weighting is not applied.

### Layer 1 — File-manifest pass (`internal/walk`)

- Single bounded parallel walk (`GOMAXPROCS`, maxDepth 8).
- SKIP_DIRS: `node_modules, .git, dist, build, target, vendor, .venv, __pycache__, .next, coverage, .idea, .vscode, bin, obj, Pods, .terraform`.
- Honor `.gitignore`; parse/read errors fail open with warnings.
- Cache `{path, ext, size}` plus:
  - `extCounts map[string]int`;
  - `nameIndex map[basename][]string` for exact basename lookup;
  - `dirIndex map[dirpath][]string` for directory/prefix detection.
- Flag manifests/content candidates during the walk; later layers read only flagged/bounded targets.

### Layer 2 — Manifest/package observations (`detect/manifest.go`)

- Parse flagged manifests only.
- `package.json` / `composer.json`: JSON.
- `pyproject.toml` / `Cargo.toml`: TOML.
- `go.mod`: `modfile.Parse` / `ParseLax`.
- `requirements.txt` and supported Gemfile subset: bounded line-oriented parsing.
- Emit **every declared dependency as an Observation**, even when no local DetectionRule knows it. Example:
  `Observation{Key:"pkg:npm:react", Kind:ObsPackage, Value:"react", Evidence:["package.json"], Layer:2, Member:"apps/web"}`.
- Known package exact/pattern rules map those observations to technology Signals at the wave barrier; unknown observations remain available for P3.

### Layer 3 — Extension, filename, and directory observations (`detect/files.go`)

- `ExtensionRule{Extension, MinCount}` consumes `extCounts` with no file reads.
- Exact `FileRule`s use `nameIndex` O(1).
- Glob `FileRule`s (`next.config.*`, etc.) evaluate against **sorted basename keys** for deterministic matching; no `stat` calls.
- `dirIndex` handles `.github/workflows/`, `docs/`, and other prefix rules.
- Emit raw observations first; local detection matcher produces Signals.

### Layer 4 — Content-pattern observations (`detect/content.go`, Wave 2)

- Runs after Wave-1 matching is complete.
- **Explicit configured ContentRules always run** on their target classes, even if layers 2–3 already recognized the technology, because content evidence may corroborate confidence.
- Optional/generic fallback content probes run only for targets still unresolved by layers 2–3.
- Default read cap 4KB; verbose build formats such as Gradle / `.csproj` may use 16KB. Per-rule `ReadLimit` may lower/raise within a global hard maximum.
- NUL in first 512B => binary skip. Regexes precompiled once. Layout-aware helpers remain allowed for Gradle/.NET multi-module forms.

### Layer 5 — Context observations (`detect/context.go`)

- Bounded README, package description, docs filename/context hints -> docs/marketing/seo/ci observations.
- Base confidence 0.4 when mapped to a known context technology.
- Context/extension observations may enrich P3 queries but do not by themselves trigger external discovery.

### Layer 6 — Deterministic known-signal merge (`detect/merge.go`)

- Detector layers write fixed slots; merge consumes mapped Signals in fixed order `2 -> 3 -> 5 -> 4`.
- First de-dup by `(technology key, scope)`; then global by technology key.
- Reasons/evidence deterministic union; alphabetical tiebreak within one layer/scope.
- Per-scope confidence: `min(1.0, highest_base + 0.1*(agreeing_layers-1))`.
- Member branch: compute each member's per-scope confidence, take `member_base = max(per-member confidence)`, then multiply by `0.5 + 0.5*(members_with_signal/total_members)`.
- Root branch: score root evidence independently, with no member-share multiplier.
- If both root and member branches exist for the same technology, final detector confidence is the max of the two. Members remain recorded for provenance.
- Final MergedSignals sort by `(domain,key)`; unresolved observations sort by `(kind,domain,key,member,evidence)`.

### Engine invariants

- **One walk, many bounded readers.** No detector re-walks the tree.
- **Observations are not technologies.** Unknown raw facts survive local matching when externally eligible.
- **Two-wave scheduling.** Wave 2 depends only on completed Wave-1 match results; no goroutine races influence gating.
- **Explicit content corroboration.** Configured content rules are not suppressed merely because a cheaper detector already matched.
- **Deterministic output.** Fixed slots/order, stable de-dup, stable sort, and backend-priority post-merge make repeated JSON output diff-stable.
- **Fail open for workspace evidence.** Unreadable/unparseable workspace files record warnings and skip that evidence; they do not abort the scan.
- **Fail closed for install structure.** Invalid SkillRef/source/agent syntax never reaches `exec`.
- **Source-qualified identity.** No component assumes skill name alone is globally unique.
- **Lock separation.** Skillscan state never overwrites the upstream `skills` CLI's own lock/state files.
