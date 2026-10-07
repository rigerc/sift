package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rigerc/sift/config"
	"github.com/rigerc/sift/internal/app"
	"github.com/rigerc/sift/internal/backend"
	"github.com/rigerc/sift/internal/backend/register"
	"github.com/rigerc/sift/internal/model"
	"github.com/rigerc/sift/internal/plan"
	"github.com/rigerc/sift/internal/prompt"
	"github.com/rigerc/sift/internal/report"
	agentreport "github.com/rigerc/sift/internal/report/agent"

	"github.com/spf13/cobra"
)

// Indirections keep machine-mode and cancellation tests independent of a TTY.
var (
	ttyDetected  = prompt.IsTTY
	selectSkills = prompt.SelectSkillsContext
)

func commandContext(c *cobra.Command) context.Context {
	if r := Runtime(); r != nil {
		return r.Context
	}
	return c.Context()
}
func cwd() (string, error) { return os.Getwd() }

func init() {
	rootCmd.AddCommand(newScanCommand(), newPlanCommand(), newAgentCommand(), newBackendsCommand())
}

type scanFlags struct {
	json, verbose, online bool
	catalog               string
	depth                 int
}

func (f *scanFlags) bind(c *cobra.Command) {
	c.Flags().BoolVar(&f.json, "json", false, "Emit structured JSON")
	c.Flags().BoolVar(&f.online, "online", true, "Enable built-in discovery backends")
	c.Flags().StringVar(&f.catalog, "catalog", "", "Local rule catalog path or file:// URL")
	c.Flags().IntVar(&f.depth, "max-depth", 8, "Maximum filesystem depth (1..64)")
}

func effectiveConfig() *config.Config {
	if r := Runtime(); r != nil {
		return r.Config.Config
	}
	cfg := config.DefaultConfig()
	if value, ok := os.LookupEnv("SIFT_CATALOG_URL"); ok {
		cfg.Scan.Catalog = value
	}
	return cfg
}

func (f scanFlags) options(c *cobra.Command) (app.ScanOptions, error) {
	cfg := effectiveConfig().Scan
	if c.Flags().Changed("catalog") {
		cfg.Catalog = f.catalog
	}
	if c.Flags().Changed("online") {
		cfg.Online = f.online
	}
	if c.Flags().Changed("max-depth") {
		cfg.MaxDepth = f.depth
	}
	if cfg.MaxDepth < 1 || cfg.MaxDepth > 64 {
		return app.ScanOptions{}, fmt.Errorf("max-depth must be between 1 and 64")
	}
	return app.ScanOptions{
		Catalog:  strings.TrimPrefix(cfg.Catalog, "file://"),
		MaxDepth: cfg.MaxDepth, Online: cfg.Online,
	}, nil
}

func planOptions(c *cobra.Command, agents []string, global bool) plan.Options {
	if !c.Flags().Changed("global") {
		global = effectiveConfig().Install.Global
	}
	return plan.Options{Agents: agents, Global: global, AllowLocal: true}
}

func newScanCommand() *cobra.Command {
	var f scanFlags
	var dry, global bool
	var agents []string
	c := &cobra.Command{
		Use: "scan [path]", Short: "Scan workspace technologies and suggest skills",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if f.json && dry {
				return fmt.Errorf("--json and --dry-run conflict")
			}
			opts, err := f.options(c)
			if err != nil {
				return err
			}
			root, err := cwd()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				root = args[0]
			}
			svc := app.Service{}
			result, err := svc.Scan(commandContext(c), root, opts)
			if err != nil {
				return err
			}
			popts := planOptions(c, agents, global)
			if dry {
				p, err := svc.BuildPlan(root, result, nil, popts)
				if err != nil {
					return err
				}
				return report.JSON(c.OutOrStdout(), p)
			}
			if f.json {
				return report.ScanJSON(c.OutOrStdout(), result, report.ScanOptions{Verbose: f.verbose, Agents: agents, Global: popts.Global})
			}
			if !ttyDetected() {
				return report.Table(c.OutOrStdout(), result, f.verbose)
			}
			selected, err := selectSkills(commandContext(c), result)
			if err != nil {
				return err
			}
			if len(selected) == 0 {
				return nil
			}
			p, err := svc.BuildPlan(root, result, selected, popts)
			if err != nil {
				return err
			}
			return report.PlanText(c.OutOrStdout(), p)
		},
	}
	f.bind(c)
	c.Flags().BoolVar(&f.verbose, "verbose", false, "Include hidden suggestions and evidence")
	c.Flags().BoolVar(&dry, "dry-run", false, "Emit a JSON plan for recommended local skills; never execute")
	c.Flags().BoolVar(&global, "global", false, "Plan for user scope instead of project scope")
	c.Flags().StringSliceVar(&agents, "agent", nil, "Target agent IDs (comma-separated or repeated)")
	return c
}

func newPlanCommand() *cobra.Command {
	var names, agents []string
	var global, jsonOut bool
	c := &cobra.Command{
		Use: "plan <source>", Short: "Generate copyable commands for explicitly named skills; never install",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(names) == 0 {
				return fmt.Errorf("at least one --skill is required")
			}
			root, err := cwd()
			if err != nil {
				return err
			}
			refs := make([]model.SkillRef, len(names))
			for i, name := range names {
				refs[i] = model.SkillRef{Source: args[0], Name: name}
			}
			p, err := plan.Build(root, refs, planOptions(c, agents, global))
			if err != nil {
				return err
			}
			if jsonOut {
				return report.JSON(c.OutOrStdout(), p)
			}
			return report.PlanText(c.OutOrStdout(), p)
		},
	}
	c.Flags().StringSliceVar(&names, "skill", nil, "Skill names (required; comma-separated or repeated)")
	c.Flags().StringSliceVar(&agents, "agent", nil, "Target agent IDs (comma-separated or repeated)")
	c.Flags().BoolVar(&global, "global", false, "Plan for user scope instead of project scope")
	c.Flags().BoolVar(&jsonOut, "json", false, "Emit the structured plan")
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
		options, err := f.options(c)
		if err != nil {
			return err
		}
		result, err := (app.Service{}).Scan(commandContext(c), root, options)
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

func capabilityNames(c backend.Capability) string {
	if c.Has(backend.CapSearch) {
		return "search"
	}
	return ""
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
