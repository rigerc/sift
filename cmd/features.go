package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"go-s/internal/app"
	"go-s/internal/backend"
	"go-s/internal/backend/register"
	"go-s/internal/install"
	"go-s/internal/model"
	"go-s/internal/prompt"
	"go-s/internal/report"

	"github.com/spf13/cobra"

	agentreport "go-s/internal/report/agent"
)

func isTTY() bool { return prompt.IsTTY() }

func commandContext(c *cobra.Command) context.Context {
	if r := Runtime(); r != nil {
		return r.Context
	}
	return c.Context()
}
func cwd() (string, error) { return os.Getwd() }
func init() {
	rootCmd.AddCommand(newScanCommand(), newInstallCommand(), newStatusCommand(), newUpdateCommand(), newAgentCommand(), newBackendsCommand())
}

type scanFlags struct {
	json, verbose, online bool
	catalog               string
	depth                 int
}

func (f *scanFlags) bind(c *cobra.Command) {
	c.Flags().BoolVar(&f.json, "json", false, "Emit JSON")
	c.Flags().BoolVar(&f.online, "online", false, "Enable built-in discovery backends")
	c.Flags().StringVar(&f.catalog, "catalog", "", "Local rule catalog path or file:// URL")
	c.Flags().IntVar(&f.depth, "max-depth", 8, "Maximum filesystem depth")
}

// options builds scan options. onlineChanged reports whether --online was
// explicitly passed; otherwise the configured scan.online default wins.
func (f scanFlags) options(onlineChanged bool) app.ScanOptions {
	catalog := f.catalog
	if catalog == "" {
		catalog = os.Getenv("SKILLSCAN_CATALOG_URL")
	}
	catalog = strings.TrimPrefix(catalog, "file://")
	online := f.online
	if !online && !onlineChanged {
		if r := Runtime(); r != nil {
			online = r.Config.Config.Scan.Online
		}
	}
	return app.ScanOptions{Catalog: catalog, MaxDepth: f.depth, Online: online}
}

func newScanCommand() *cobra.Command {
	var f scanFlags
	var yes, dry, interactive, tuiMode, global, allow bool
	var agents []string
	c := &cobra.Command{Use: "scan [path]", Short: "Scan workspace technologies and suggest skills", Args: cobra.MaximumNArgs(1), RunE: func(c *cobra.Command, args []string) error {
		root, err := cwd()
		if err != nil {
			return err
		}
		if len(args) == 1 {
			root = args[0]
		}
		if f.depth < 1 || f.depth > 64 {
			return fmt.Errorf("max-depth must be between 1 and 64")
		}
		if interactive && (f.json || yes || dry) {
			return fmt.Errorf("interactive conflicts with json, yes, or dry-run")
		}
		if interactive || tuiMode || tuiFlag {
			// Full-screen BubbleTea results screen; main.go launches it
			// from the pending TUIRequest. Interactive conflicts with
			// machine output and automation flags.
			tuiReq = &TUIRequest{Screen: "scan", ScanRoot: root, ScanOpts: f.options(c.Flags().Changed("online"))}
			runUI = true
			return nil
		}
		if yes {
			// Automation path: install suggested local skills without prompts.
			svc := app.Service{Output: c.ErrOrStderr()}
			result, err := svc.Scan(commandContext(c), root, f.options(c.Flags().Changed("online")))
			if err != nil {
				return err
			}
			plan, err := svc.PlanInstall(root, result, nil, install.Options{Agents: agents, Global: global, AllowLocal: true})
			if err != nil {
				return err
			}
			if err := validatePlan(commandContext(c), plan, f.options(c.Flags().Changed("online")).Online, allow); err != nil {
				return err
			}
			return svc.Install(commandContext(c), plan)
		}
		if dry {
			svc := app.Service{Output: c.ErrOrStderr()}
			result, err := svc.Scan(commandContext(c), root, f.options(c.Flags().Changed("online")))
			if err != nil {
				return err
			}
			plan, err := svc.PlanInstall(root, result, nil, install.Options{Agents: agents, Global: global, AllowLocal: true})
			if err != nil {
				return err
			}
			return report.JSON(c.OutOrStdout(), plan)
		}
		if f.json {
			svc := app.Service{Output: c.ErrOrStderr()}
			result, err := svc.Scan(commandContext(c), root, f.options(c.Flags().Changed("online")))
			if err != nil {
				return err
			}
			return report.ScanJSON(c.OutOrStdout(), result, report.ScanOptions{Verbose: f.verbose, Agents: agents, Global: global})
		}
		if !isTTY() {
			// Headless default: deterministic table for pipes and CI.
			svc := app.Service{Output: c.ErrOrStderr()}
			result, err := svc.Scan(commandContext(c), root, f.options(c.Flags().Changed("online")))
			if err != nil {
				return err
			}
			return report.Table(c.OutOrStdout(), result, f.verbose)
		}
		// TTY default: guided huh flow — summary, selection, and plan
		// confirmation. The plain table stays a headless-only renderer so
		// interactive runs never duplicate output into terminal history.
		svc := app.Service{Output: c.ErrOrStderr()}
		result, err := svc.Scan(commandContext(c), root, f.options(c.Flags().Changed("online")))
		if err != nil {
			return err
		}
		selected, err := prompt.SelectSkills(result, themeName())
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			return nil
		}
		plan, err := svc.PlanInstall(root, result, selected, install.Options{Agents: agents, Global: global, AllowLocal: true})
		if err != nil {
			return err
		}
		ok, err := prompt.ConfirmPlan(plan, themeName())
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := validatePlan(commandContext(c), plan, f.options(c.Flags().Changed("online")).Online, allow); err != nil {
			return err
		}
		return svc.Install(commandContext(c), plan)
	}}
	f.bind(c)
	c.Flags().BoolVar(&f.verbose, "verbose", false, "Include hidden suggestions and evidence")
	c.Flags().BoolVar(&interactive, "interactive", false, "Open interactive results in the full-screen TUI")
	c.Flags().BoolVar(&tuiMode, "tui", false, "Open interactive results in the full-screen TUI")
	c.Flags().BoolVar(&yes, "yes", false, "Install suggested local skills")
	c.Flags().BoolVar(&dry, "dry-run", false, "Print exact install plan without execution")
	c.Flags().BoolVar(&allow, "allow-unvalidated", false, "Permit unknown online validation results")
	c.Flags().BoolVar(&global, "global", false, "Install to user scope")
	c.Flags().StringSliceVar(&agents, "agent", nil, "Target agent IDs (comma-separated or repeated)")
	return c
}

func newInstallCommand() *cobra.Command {
	var names, agents []string
	var yes, dry, global, online, allow, tuiMode bool
	c := &cobra.Command{Use: "install <source>", Short: "Install named skills through the npx skills CLI", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if len(names) == 0 {
			return fmt.Errorf("at least one --skill is required")
		}
		if (tuiMode || tuiFlag) && (yes || dry) {
			return fmt.Errorf("tui conflicts with yes or dry-run")
		}
		root, err := cwd()
		if err != nil {
			return err
		}
		refs := make([]model.SkillRef, len(names))
		for i, n := range names {
			refs[i] = model.SkillRef{Source: args[0], Name: n}
		}
		p, err := install.Build(root, refs, install.Options{Agents: agents, Global: global, AllowLocal: true})
		if err != nil {
			return err
		}
		if dry {
			return report.JSON(c.OutOrStdout(), p)
		}
		if tuiMode || tuiFlag {
			// Full-screen BubbleTea install screen; main.go launches it.
			tuiReq = &TUIRequest{Screen: "install", Plan: p}
			runUI = true
			return nil
		}
		if !yes {
			if !isTTY() {
				return fmt.Errorf("non-interactive install requires --yes or --dry-run")
			}
			ok, err := prompt.ConfirmPlan(p, themeName())
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
		if err := validatePlan(commandContext(c), p, online, allow); err != nil {
			return err
		}
		return (app.Service{Output: c.ErrOrStderr()}).Install(commandContext(c), p)
	}}
	c.Flags().StringSliceVar(&names, "skill", nil, "Skill names (comma-separated or repeated)")
	c.Flags().StringSliceVar(&agents, "agent", nil, "Agent IDs")
	c.Flags().BoolVar(&yes, "yes", false, "Confirm installation")
	c.Flags().BoolVar(&dry, "dry-run", false, "Print plan without execution")
	c.Flags().BoolVar(&global, "global", false, "Use user scope")
	c.Flags().BoolVar(&online, "online", false, "Run online validation")
	c.Flags().BoolVar(&allow, "allow-unvalidated", false, "Permit unknown online validation")
	c.Flags().BoolVar(&tuiMode, "tui", false, "Confirm in the full-screen TUI")
	return c
}

func newStatusCommand() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{Use: "status", Short: "Compare tracked skill content with installed files", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		root, err := cwd()
		if err != nil {
			return err
		}
		entries, err := (app.Service{}).Status(root)
		if err != nil {
			return err
		}
		if jsonOut {
			return report.JSON(c.OutOrStdout(), entries)
		}
		for _, e := range entries {
			if _, err := fmt.Fprintf(c.OutOrStdout(), "%s\t%s\t%s\t%s\n", e.Status, report.Plain(e.Source), report.Plain(e.Name), e.Scope); err != nil {
				return err
			}
		}
		return nil
	}}
	c.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON")
	return c
}

func newUpdateCommand() *cobra.Command {
	var yes, global, project bool
	c := &cobra.Command{Use: "update [name...]", Short: "Delegate skill updates to the npx skills CLI", RunE: func(c *cobra.Command, args []string) error {
		if !yes {
			return fmt.Errorf("update requires --yes")
		}
		root, err := cwd()
		if err != nil {
			return err
		}
		return install.UpdateSelected(commandContext(c), root, install.UpdateOptions{Names: args, Global: global, Project: project}, nil, c.ErrOrStderr())
	}}
	c.Flags().BoolVar(&yes, "yes", false, "Confirm updates")
	c.Flags().BoolVar(&global, "global", false, "Update user scope only")
	c.Flags().BoolVar(&project, "project", false, "Update project scope only")
	return c
}

func newAgentCommand() *cobra.Command {
	var f scanFlags
	var opts agentreport.Options
	c := &cobra.Command{Use: "agent [path]", Short: "Emit a deterministic advisory assessment for an AI agent", Args: cobra.MaximumNArgs(1), RunE: func(c *cobra.Command, args []string) error {
		root, err := cwd()
		if err != nil {
			return err
		}
		if len(args) == 1 {
			root = args[0]
		}
		result, err := (app.Service{}).Scan(commandContext(c), root, f.options(c.Flags().Changed("online")))
		if err != nil {
			return err
		}
		if f.json {
			return agentreport.JSON(c.OutOrStdout(), result, opts)
		}
		return agentreport.Markdown(c.OutOrStdout(), result, opts)
	}}
	f.bind(c)
	c.Flags().StringVar(&opts.Bucket, "bucket", "all", "Include all or suggested skills")
	c.Flags().IntVar(&opts.MaxSignals, "max-signals", 0, "Cap signals; zero means all")
	c.Flags().IntVar(&opts.ContextLines, "context-lines", 0, "Maximum evidence entries per signal; zero means all")
	c.Flags().BoolVar(&opts.NoInstructions, "no-instructions", false, "Emit assessment data only")
	return c
}

// validatePlan is shared by direct installation and scan automation. Without
// --online the plan's structural validation is authoritative; with --online the
// first authoritative validator verdict wins and unknown results require
// --allow-unvalidated. The built-in registry currently ships search-only
// backends, so validation degrades to a no-op until a validator ships.
func validatePlan(ctx context.Context, p install.Plan, online bool, allow bool) error {
	if !online {
		return nil
	}
	registry, err := register.New()
	if err != nil {
		return err
	}
	if len(registry.ByCap(backend.CapValidate)) == 0 {
		return nil
	}
	refs := []model.SkillRef{}
	for _, b := range p.Batches {
		for _, name := range b.Skills {
			refs = append(refs, model.SkillRef{Source: b.Source, Name: name})
		}
	}
	if len(refs) == 0 {
		return nil
	}
	verdicts := registry.Validate(ctx, refs)
	var invalid, unknown []string
	for _, ref := range refs {
		key := ref.Key()
		v, ok := verdicts[key]
		switch {
		case !ok || v.Status == backend.StatusUnknown:
			unknown = append(unknown, key)
		case v.Status == backend.StatusInvalid:
			invalid = append(invalid, key)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("online validation rejected: %s", strings.Join(invalid, ", "))
	}
	if len(unknown) > 0 && !allow {
		return fmt.Errorf("online validation returned unknown for: %s (use --allow-unvalidated)", strings.Join(unknown, ", "))
	}
	return nil
}

func capabilityNames(c backend.Capability) string {
	names := []string{}
	for name, cap := range map[string]backend.Capability{
		"search": backend.CapSearch, "validate": backend.CapValidate,
	} {
		if c.Has(cap) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// probeBackend builds a configured built-in adapter and runs its health
// probe, used to report availability before a scan needs it.
func probeBackend(ctx context.Context, cfg backend.Config) (string, error) {
	factory, ok := backend.FactoryFor(cfg.Type)
	if !ok {
		return "", fmt.Errorf("backend type %q is not registered", cfg.Type)
	}
	adapter, err := factory(cfg)
	if err != nil {
		return "", err
	}
	prober, ok := adapter.(backend.VersionProber)
	if !ok {
		return "", fmt.Errorf("backend %s does not support health probing", cfg.Type)
	}
	return prober.Probe(ctx)
}

func newBackendsCommand() *cobra.Command {
	var jsonOut bool
	list := &cobra.Command{Use: "list", Short: "List the built-in discovery backends", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		type row struct {
			Name, Type, Capabilities string
		}
		rows := make([]row, 0, len(register.Builtins()))
		for _, b := range register.Builtins() {
			rows = append(rows, row{Name: b.Name, Type: b.Type, Capabilities: capabilityNames(b.Capabilities)})
		}
		if jsonOut {
			return report.JSON(c.OutOrStdout(), rows)
		}
		for _, r := range rows {
			if _, err := fmt.Fprintf(c.OutOrStdout(), "%s\t%s\t%s\n", r.Name, r.Type, r.Capabilities); err != nil {
				return err
			}
		}
		return nil
	}}
	list.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON")
	check := &cobra.Command{Use: "check", Short: "Verify built-in backend availability and registry health", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		type row struct {
			Name, Type, Status, Version string
		}
		builtins := register.Builtins()
		rows := make([]row, 0, len(builtins))
		for _, cfg := range builtins {
			r := row{Name: cfg.Name, Type: cfg.Type, Status: "unavailable", Version: "-"}
			version, err := probeBackend(commandContext(c), cfg)
			if version != "" {
				r.Version = version
			}
			if err == nil {
				r.Status = "ready"
			}
			rows = append(rows, r)
		}
		if jsonOut {
			return report.JSON(c.OutOrStdout(), rows)
		}
		for _, r := range rows {
			if _, err := fmt.Fprintf(c.OutOrStdout(), "%s\t%s\t%s\t%s\n", r.Name, r.Type, r.Status, r.Version); err != nil {
				return err
			}
		}
		return nil
	}}
	check.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON")
	c := &cobra.Command{Use: "backends", Short: "Inspect discovery backends"}
	c.AddCommand(list, check)
	return c
}
