package report

import (
	"io"
	"strings"

	"go-s/internal/model"
	"go-s/internal/plan"
)

// ScanSchemaVersion identifies the machine-readable scan envelope contract.
// Agents should branch on this value before parsing, because the envelope is a
// deliberate breaking change from the raw model.ScanResult shape.
const (
	ScanSchemaVersion = "1"
	ScanKind          = "skillscan.scan"
)

// ScanOptions controls the scan envelope. Compact is the default; Verbose
// restores full signals, observations, and evidence for debugging.
type ScanOptions struct {
	Verbose bool
	Agents  []string
	Global  bool
}

type (
	// ScanEnvelope is the agent-facing scan document. Empty collections always
	// serialize as [] so consumers never need null checks.
	ScanEnvelope struct {
		SchemaVersion string              `json:"schemaVersion"`
		Kind          string              `json:"kind"`
		Root          string              `json:"root"`
		Members       []string            `json:"members"`
		Summary       ScanSummary         `json:"summary"`
		Signals       []ScanSignal        `json:"signals"`
		Suggestions   []ScanSuggestion    `json:"suggestions"`
		Unresolved    []ScanObservation   `json:"unresolved"`
		Install       *plan.Plan          `json:"install,omitempty"`
		Warnings      []ScanWarning       `json:"warnings"`
		Observations  []model.Observation `json:"observations,omitempty"`
	}
	ScanSummary struct {
		Signals    int `json:"signals"`
		Unresolved int `json:"unresolved"`
		Suggested  int `json:"suggested"`
		Possible   int `json:"possible"`
		External   int `json:"external"`
		Hidden     int `json:"hidden"`
		Total      int `json:"total"`
		Warnings   int `json:"warnings"`
	}
	// ScanSignal keeps the fields an agent needs to reason about a detected
	// technology. Detail is verbose-only.
	ScanSignal struct {
		Key          string   `json:"key"`
		Domain       string   `json:"domain"`
		Confidence   float64  `json:"confidence"`
		Reasons      []string `json:"reasons,omitempty"`
		Evidence     []string `json:"evidence,omitempty"`
		Members      []string `json:"members,omitempty"`
		Layers       []int    `json:"layers,omitempty"`
		RootObserved bool     `json:"rootObserved,omitempty"`
	}
	ScanObservation struct {
		Key      string   `json:"key"`
		Value    string   `json:"value,omitempty"`
		Kind     string   `json:"kind"`
		Member   string   `json:"member,omitempty"`
		Domain   string   `json:"domain,omitempty"`
		Reason   string   `json:"reason,omitempty"`
		Evidence []string `json:"evidence,omitempty"`
		Layer    int      `json:"layer,omitempty"`
	}
	// ScanSuggestion unifies local and external results behind one score field
	// with an explicit scoreType scale, plus install affordances.
	ScanSuggestion struct {
		Source         string   `json:"source"`
		Name           string   `json:"name"`
		Bucket         string   `json:"bucket"`
		Score          float64  `json:"score"`
		ScoreType      string   `json:"scoreType"`
		SourceKind     string   `json:"sourceKind"`
		Installable    bool     `json:"installable"`
		InstallCommand string   `json:"installCommand,omitempty"`
		URL            string   `json:"url,omitempty"`
		Backend        string   `json:"backend,omitempty"`
		Reason         string   `json:"reason,omitempty"`
		Description    string   `json:"description,omitempty"`
		Stale          bool     `json:"stale,omitempty"`
		Confidence     float64  `json:"confidence,omitempty"`
		ExternalScore  float64  `json:"externalScore,omitempty"`
		Reasons        []string `json:"reasons,omitempty"`
		Evidence       []string `json:"evidence,omitempty"`
		Technologies   []string `json:"technologies,omitempty"`
		Members        []string `json:"members,omitempty"`
	}
	ScanWarning struct {
		Source  string `json:"source"`
		Message string `json:"message"`
	}
)

// BuildScan projects a scan result onto the versioned envelope. An install
// plan for the default "suggested" selection is attached when it can be built;
// a build failure degrades to a structured warning instead of failing the
// scan.
func BuildScan(result model.ScanResult, opts ScanOptions) ScanEnvelope {
	agents := opts.Agents
	if len(agents) == 0 {
		agents = plan.DetectAgents(result.Root)
	}
	env := ScanEnvelope{
		SchemaVersion: ScanSchemaVersion,
		Kind:          ScanKind,
		Root:          result.Root,
		Members:       stringsOrEmpty(result.Members),
		Signals:       make([]ScanSignal, 0, len(result.Signals)),
		Suggestions:   make([]ScanSuggestion, 0, len(result.Suggestions)),
		Unresolved:    make([]ScanObservation, 0, len(result.Unresolved)),
		Warnings:      make([]ScanWarning, 0, len(result.Warnings)),
	}
	for _, signal := range result.Signals {
		env.Signals = append(env.Signals, scanSignal(signal, opts.Verbose))
	}
	for _, suggestion := range result.Suggestions {
		env.Suggestions = append(env.Suggestions, scanSuggestion(suggestion, agents, opts.Global, opts.Verbose))
	}
	for _, observation := range result.Unresolved {
		env.Unresolved = append(env.Unresolved, scanObservation(observation, opts.Verbose))
	}
	for _, warning := range result.Warnings {
		env.Warnings = append(env.Warnings, scanWarning(warning))
	}
	if opts.Verbose {
		env.Observations = observationsOrEmpty(result.Observations)
	}
	env.Install, env.Warnings = scanInstallPlan(result, agents, opts, env.Warnings)
	env.Summary = summarize(result, env)
	return env
}

// ScanJSON writes the versioned scan envelope.
func ScanJSON(w io.Writer, result model.ScanResult, opts ScanOptions) error {
	return JSON(w, BuildScan(result, opts))
}

func scanSignal(signal model.MergedSignal, verbose bool) ScanSignal {
	out := ScanSignal{Key: signal.Key, Domain: signal.Domain, Confidence: signal.Confidence}
	if verbose {
		out.Reasons = stringsOrEmpty(signal.Reasons)
		out.Evidence = stringsOrEmpty(signal.Evidence)
		out.Members = stringsOrEmpty(signal.Members)
		out.Layers = intsOrEmpty(signal.Layers)
		out.RootObserved = signal.RootObserved
	}
	return out
}

func scanObservation(observation model.Observation, verbose bool) ScanObservation {
	out := ScanObservation{
		Key:    observation.Key,
		Value:  observation.Value,
		Kind:   string(observation.Kind),
		Member: observation.Member,
	}
	if verbose {
		out.Domain = observation.Domain
		out.Reason = observation.Reason
		out.Evidence = stringsOrEmpty(observation.Evidence)
		out.Layer = observation.Layer
	}
	return out
}

func scanSuggestion(suggestion model.Suggestion, agents []string, global, verbose bool) ScanSuggestion {
	out := ScanSuggestion{
		Source:      suggestion.Skill.Source,
		Name:        suggestion.Skill.Name,
		Bucket:      suggestion.Bucket,
		SourceKind:  plan.SourceKind(suggestion.Skill.Source),
		URL:         suggestion.URL,
		Backend:     suggestion.SourceBackend,
		Reason:      strings.Join(suggestion.Reasons, "; "),
		Description: firstNonEmpty(suggestion.Evidence),
		Stale:       suggestion.Stale,
	}
	if suggestion.Bucket == "external" {
		out.Score = suggestion.ExternalScore
		out.ScoreType = "external"
	} else {
		out.Score = suggestion.Confidence
		out.ScoreType = "confidence"
	}
	p, err := plan.Build(".", []model.SkillRef{suggestion.Skill}, plan.Options{Agents: agents, Global: global, AllowLocal: suggestion.Bucket != "external" && suggestion.SourceBackend == ""})
	if err == nil {
		out.Installable = true
		out.InstallCommand = "npx " + shellJoin(p.Batches[0].Argv)
	}
	if verbose {
		out.Confidence = suggestion.Confidence
		out.ExternalScore = suggestion.ExternalScore
		out.Reasons = stringsOrEmpty(suggestion.Reasons)
		out.Evidence = stringsOrEmpty(suggestion.Evidence)
		out.Technologies = stringsOrEmpty(suggestion.Technologies)
		out.Members = stringsOrEmpty(suggestion.Members)
	}
	return out
}

// scanInstallPlan attaches an advisory plan for the default recommended local
// selection. Errors degrade to a warning; no commands are ever executed.
func scanInstallPlan(result model.ScanResult, agents []string, opts ScanOptions, warnings []ScanWarning) (*plan.Plan, []ScanWarning) {
	refs := make([]model.SkillRef, 0, len(result.Suggestions))
	for _, suggestion := range result.Suggestions {
		if suggestion.Bucket == "suggested" && suggestion.SourceBackend == "" {
			refs = append(refs, suggestion.Skill)
		}
	}
	if len(refs) == 0 {
		return nil, warnings
	}
	plan, err := plan.Build(result.Root, refs, plan.Options{Agents: agents, Global: opts.Global, AllowLocal: true})
	if err != nil {
		return nil, append(warnings, ScanWarning{Source: "install", Message: err.Error()})
	}
	return &plan, warnings
}

func scanWarning(message string) ScanWarning {
	if rest, ok := strings.CutPrefix(message, "backend discovery: "); ok {
		return ScanWarning{Source: "discovery", Message: rest}
	}
	return ScanWarning{Source: "scan", Message: message}
}

func summarize(result model.ScanResult, env ScanEnvelope) ScanSummary {
	summary := ScanSummary{
		Signals:    len(result.Signals),
		Unresolved: len(result.Unresolved),
		Total:      len(result.Suggestions),
		Warnings:   len(env.Warnings),
	}
	for _, suggestion := range result.Suggestions {
		switch suggestion.Bucket {
		case "suggested":
			summary.Suggested++
		case "possible":
			summary.Possible++
		case "external":
			summary.External++
		case "hidden":
			summary.Hidden++
		}
	}
	return summary
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringsOrEmpty(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return values
}

func intsOrEmpty(values []int) []int {
	if len(values) == 0 {
		return []int{}
	}
	return values
}

func observationsOrEmpty(values []model.Observation) []model.Observation {
	if len(values) == 0 {
		return []model.Observation{}
	}
	return values
}

// shellJoin renders argv for display. Arguments containing shell
// metacharacters are single-quoted so the command is copy-pasteable.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\n\"'\\$`&|;<>(){}[]*?!#~") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}
