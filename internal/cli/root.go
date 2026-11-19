package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/domainreseller/cli/internal/ops"
)

const rootLong = `dna is the command line for the DomainNameAPI reseller API: search, register,
renew and transfer domains, manage DNS, DNSSEC, SSL certificates and hosting.

It is built to be driven by people and by AI agents alike:
  - every command prints JSON when piped (or with -o json), tables on a terminal
  - errors are JSON on stderr with stable exit codes (see "dna agent guide")
  - billable and destructive commands require --yes; --dry-run shows the exact request
  - "dna schema" describes every operation as JSON Schema, "dna mcp serve" is an MCP server

Get started:
  dna auth login --env ote        # test environment first (separate OTE API key)
  dna domain search example.com
  dna domain list --all -o json`

func (a *App) newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "dna",
		Short:         "DomainNameAPI CLI: domains, DNS, SSL and hosting for humans and AI agents",
		Long:          rootLong,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       a.Build.Version,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: false,
		},
	}
	root.SetVersionTemplate("dna {{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageErr(err) })
	root.Args = cobra.NoArgs
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		// A person at a terminal gets the interactive shell; scripts and
		// agents (and the shell itself) get the plain help text.
		if a.canRunShell() {
			return a.runShell(cmd.Context())
		}
		return cmd.Help()
	}
	a.setupHelp(root)
	addGlobalFlags(root, &a.flags)

	root.AddGroup(&cobra.Group{ID: "api", Title: "API commands:"}, &cobra.Group{ID: "cli", Title: "CLI and AI commands:"})

	groups := map[string]*cobra.Command{}
	getGroup := func(path []string) *cobra.Command {
		parent := root
		for i := range path {
			key := strings.Join(path[:i+1], " ")
			if g, ok := groups[key]; ok {
				parent = g
				continue
			}
			short := ops.Groups()[key]
			if short == "" {
				short = subgroupHelp[key]
			}
			g := &cobra.Command{Use: path[i], Short: short, Args: cobra.NoArgs, RunE: groupRunE}
			if parent == root {
				g.GroupID = "api"
			}
			parent.AddCommand(g)
			groups[key] = g
			parent = g
		}
		return parent
	}
	for _, op := range ops.All() {
		path := op.CommandPath()
		parent := getGroup(path[:len(path)-1])
		parent.AddCommand(a.newOpCommand(op))
	}

	for _, c := range []*cobra.Command{
		a.newAuthCmd(), a.newConfigCmd(), a.newAPICmd(), a.newSchemaCmd(), a.newEnumsCmd(),
		a.newAgentCmd(), a.newMCPCmd(), a.newDoctorCmd(), a.newVersionCmd(),
	} {
		c.GroupID = "cli"
		root.AddCommand(c)
	}
	root.AddCommand(a.newDocsCmd())
	root.SetHelpCommandGroupID("cli")
	root.SetCompletionCommandGroupID("cli")
	return root
}

// groupRunE shows help for a bare group command, or fails on an unknown subcommand.
func groupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	return cmd.Help()
}
