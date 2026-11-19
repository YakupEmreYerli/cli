package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/domainreseller/cli/internal/mcp"
	"github.com/domainreseller/cli/internal/ops"
)

// ---------------------------------------------------------------- agent

func (a *App) newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Usage guide and skill file for AI coding agents",
		Long: `Helpers for AI agents (Claude Code, Codex, Cursor, Gemini CLI, ...).

  dna agent guide           prints a compact, always up-to-date usage guide
  dna agent skill --install installs the dna skill for Claude Code

An agent that has never seen dna can run "dna agent guide" once and know how
to drive every command safely.`,
		Args: cobra.NoArgs,
		RunE: groupRunE,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "guide",
		Short: "Print the agent usage guide (markdown)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if a.flags.output == "json" {
				return a.printJSON(map[string]any{"guide": AgentGuide(), "exitCodes": ExitCodes})
			}
			fmt.Fprint(a.Out, AgentGuide())
			return nil
		},
	})
	var install bool
	var dir string
	skill := &cobra.Command{
		Use:   "skill",
		Short: "Print or install the dna Agent Skill (SKILL.md)",
		Long: `Prints the dna Agent Skill. With --install it is written to
~/.claude/skills/dna/SKILL.md, where Claude Code discovers it automatically;
use --dir for other agents or a project-level .claude/skills directory.`,
		Example: `  dna agent skill --install
  dna agent skill --install --dir .claude/skills   # project skill, commit it with your repo`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			content := SkillFile()
			if !install {
				fmt.Fprint(a.Out, content)
				return nil
			}
			base := dir
			if base == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				base = filepath.Join(home, ".claude", "skills")
			}
			path := filepath.Join(base, "dna", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
			if a.machineOutput() {
				return a.printJSON(map[string]any{"installed": path})
			}
			fmt.Fprintf(a.Out, "Installed the dna skill at %s\n", path)
			return nil
		},
	}
	skill.Flags().BoolVar(&install, "install", false, "Write the skill to disk instead of printing it")
	skill.Flags().StringVar(&dir, "dir", "", "Skills directory (default ~/.claude/skills)")
	cmd.AddCommand(skill)
	return cmd
}

// AgentGuide renders the agent usage guide from the operation registry.
func AgentGuide() string {
	var b strings.Builder
	b.WriteString(`# dna CLI - guide for AI agents

dna drives the DomainNameAPI reseller API (domains, DNS, DNSSEC, SSL, hosting).
This guide is generated from the CLI itself, so it matches the installed version.

## Rules

1. **Environment.** ` + "`--env ote`" + ` (or ` + "`--ote`" + `) is the test environment, the default is production (real money).
   Prefer OTE while experimenting. ` + "`dna auth status -o json`" + ` shows what is in effect.
2. **Output.** Piped output is JSON. Pass ` + "`-o json`" + ` explicitly to be safe; use ` + "`--jq`" + ` to select fields
   (` + "`--jq .items[].domainName`" + `). Errors are a JSON object ` + "`{\"error\":{...}}`" + ` on stderr.
   The colored terminal UI is for people only: when ` + "`AI_AGENT`" + `, ` + "`CLAUDECODE`" + ` or ` + "`DNA_AGENT=1`" + ` is set, dna prints
   JSON, never prompts and shows no spinner, even in a pseudo-terminal.
3. **Safety classes.** Every command is ` + "`read`" + `, ` + "`write`" + `, ` + "`destructive`" + ` or ` + "`billable`" + ` (see the table).
   Billable and destructive commands fail with exit code 9 unless ` + "`--yes`" + ` is given.
   **Never add --yes on your own:** show the user the plan (` + "`--dry-run`" + ` prints the exact request), get explicit approval, then re-run with ` + "`--yes`" + `.
4. **Read-only mode.** ` + "`--read-only`" + ` or ` + "`DNA_READ_ONLY=1`" + ` blocks every change (exit code 10). Use it for exploration.
5. **Inputs.** Positional args and flags are listed in ` + "`dna <command> --help`" + `. ` + "`dna schema <operation>`" + ` prints the full
   JSON Schema plus an example; the same JSON can be passed with ` + "`--body @file.json`" + ` or ` + "`--body -`" + ` (stdin).
   Inputs are validated before anything is sent (exit code 2 with a field list).
6. **Coded values.** Statuses and types are numbers in responses; ` + "`dna enums <Name>`" + ` explains them. Flags accept names
   (` + "`--dnssec-mode DisableDnssec`" + `, ` + "`--status-ids Active`" + `).
7. **Lists** are paged: ` + "`--max-result-count`" + ` / ` + "`--skip-count`" + `, or ` + "`--all`" + ` for everything.
8. **Retries.** The CLI already retries rate limits (429) and transient errors on reads. Never blindly retry a billable
   command after a timeout or exit code 8: check the result first (e.g. ` + "`dna domain info <domain>`" + `).
9. **Typical flows**
   - Register: ` + "`domain search`" + ` -> ` + "`contact list`" + ` (saved handles; the default is used when ` + "`--handle`" + ` is left out,
     the platform nameservers when ` + "`--ns`" + ` is) -> (` + "`contact create`" + ` if there is none; it becomes the default) ->
     ` + "`domain register --dry-run`" + ` -> approval -> ` + "`--yes`" + `. Ask the user which contact to use when it is not obvious.
   - Renew: ` + "`domain renew-check`" + ` -> approval -> ` + "`domain renew --period N --yes`" + `.
   - Transfer in: ` + "`domain transfer-check <domain> <authcode>`" + ` -> approval -> ` + "`domain transfer --yes`" + ` -> ` + "`domain transfer-status`" + `.
   - DNS: ` + "`dns list`" + `, ` + "`dns add --name www --type A --content 192.0.2.1`" + `, then ` + "`dns apply`" + ` if the domain is not on the platform nameservers.
   - SSL: ` + "`product ssls`" + ` -> ` + "`ssl initialize`" + ` (billable) -> ` + "`ssl generate-csr`" + ` -> ` + "`ssl configure`" + ` -> ` + "`ssl status`" + ` -> ` + "`ssl content`" + `.

## Exit codes

| code | name | meaning |
|---:|---|---|
`)
	for _, e := range ExitCodes {
		fmt.Fprintf(&b, "| %d | %s | %s |\n", e.Code, e.Name, e.Meaning)
	}
	b.WriteString(`
Error JSON (stderr): ` + "`{\"error\":{\"kind\",\"code\",\"message\",\"status\",\"fields\",\"hint\",\"correlationId\",\"retryable\",\"exitCode\"}}`" + `

## Commands

| command | safety | what it does |
|---|---|---|
`)
	for _, op := range ops.All() {
		cmd := "dna " + strings.Join(op.CommandPath(), " ")
		for _, arg := range op.Args {
			cmd += " <" + argName(strings.TrimSuffix(arg, "...")) + ">"
			if strings.HasSuffix(arg, "...") {
				cmd += "..."
			}
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", cmd, op.Safety, op.Summary)
	}
	b.WriteString(`
Contact book (on this computer, the API cannot list handles): ` + "`dna contact list [--scan]`" + `, ` + "`dna contact default [handle]`" + `,
` + "`dna contact forget <handle>`" + `.
Other commands: ` + "`dna auth login|status|logout`" + `, ` + "`dna config show|use|set`" + `, ` + "`dna api <path>`" + ` (raw request),
` + "`dna schema [op]`" + `, ` + "`dna enums [name]`" + `, ` + "`dna doctor`" + `, ` + "`dna mcp serve`" + ` (MCP server), ` + "`dna version`" + `.
`)
	return b.String()
}

// SkillFile renders the Agent Skill (SKILL.md) for dna.
func SkillFile() string {
	return `---
name: dna
description: Manage domains, DNS records, DNSSEC, SSL certificates and hosting through the DomainNameAPI reseller API with the dna CLI. Use when the user wants to search, register, renew or transfer domains, change nameservers or DNS records, check balances, prices or TLD rules, or manage SSL/hosting on DomainNameAPI (domainresellerapi.com).
---

` + AgentGuide()
}

// ---------------------------------------------------------------- mcp

func (a *App) newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run dna as an MCP server for AI assistants",
		Long: `dna includes a Model Context Protocol server (stdio). Every API operation
becomes a tool with a JSON Schema and safety annotations; billable and
destructive tools only run when called with confirm=true.

  dna mcp serve            start the server (what your MCP client runs)
  dna mcp config           print config snippets for MCP clients
  dna mcp tools            list the tools as JSON`,
		Args: cobra.NoArgs,
		RunE: groupRunE,
	}

	var noBillable bool
	var groups []string
	newServer := func() (*mcp.Server, error) {
		r, err := a.resolved()
		if err != nil {
			return nil, err
		}
		return mcp.New(mcp.Options{
			Client:      a.client,
			Environment: string(r.Environment),
			BaseURL:     r.BaseURL,
			Version:     a.Build.Version,
			ReadOnly:    r.ReadOnly,
			NoBillable:  noBillable,
			Groups:      groups,
			Log:         a.Err,
		}), nil
	}
	addServerFlags := func(c *cobra.Command) {
		c.Flags().BoolVar(&noBillable, "no-billable", false, "Do not expose billable tools (register, renew, transfer, ...)")
		c.Flags().StringSliceVar(&groups, "groups", nil, "Only expose these groups: "+strings.Join(sortedGroupNames(), ","))
	}

	serve := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server on stdin/stdout",
		Long: `Starts the MCP server on stdio. Configure your client to run "dna mcp serve".

Credentials come from the usual places (profile, DNA_RESELLER_ID / DNA_API_KEY,
--env / DNA_ENV). Combine with --read-only to give an assistant safe,
look-only access, or --no-billable to keep purchases out of reach.`,
		Example: `  claude mcp add dna -- dna mcp serve --env ote
  dna mcp serve --read-only
  dna mcp serve --groups domain,dns --no-billable`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := newServer()
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			err = s.Serve(ctx, a.In, a.Out)
			if err == context.Canceled {
				return nil
			}
			return err
		},
	}
	addServerFlags(serve)

	tools := &cobra.Command{
		Use:   "tools",
		Short: "List the MCP tools as JSON",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			s, err := newServer()
			if err != nil {
				return err
			}
			return a.printJSON(s.Tools())
		},
	}
	addServerFlags(tools)

	var client string
	var fullPath bool
	config := &cobra.Command{
		Use:   "config",
		Short: "Print MCP client configuration for dna",
		Long: "Prints how to register dna with an AI agent (MCP client): the command to run or the snippet for its config file.\n\n" +
			"Clients: " + strings.Join(agentIDs(), ", ") + " (default: all).\n\n" +
			"--env, --profile, --read-only and --no-billable are added to the server command. --full-path uses the absolute path of " +
			"the dna program, for apps that start with a different PATH than your terminal (Claude Desktop on macOS, for example).",
		Example: `  dna mcp config --client claude-code --env ote
  dna mcp config --client cursor --env ote --read-only
  dna mcp config --client claude-desktop --full-path`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			env := a.flags.env
			if a.flags.ote {
				env = "ote"
			}
			launch := mcpLaunchFor(env, a.flags.profile, a.flags.readOnly, noBillable)
			if fullPath {
				launch.Bin = dnaPath()
			}
			text, err := mcpConfigText(client, launch)
			if err != nil {
				return usageErr(err)
			}
			fmt.Fprint(a.Out, text)
			return nil
		},
	}
	config.Flags().StringVar(&client, "client", "", "MCP client to print config for: "+strings.Join(agentIDs(), ", "))
	config.Flags().BoolVar(&noBillable, "no-billable", false, "Add --no-billable to the server command")
	config.Flags().BoolVar(&fullPath, "full-path", false, "Use the absolute path of the dna program")

	cmd.AddCommand(serve, tools, config)
	return cmd
}

// mcpLaunchFor is the server command for the given settings.
func mcpLaunchFor(env, profile string, readOnly, noBillable bool) mcpLaunch {
	args := []string{"mcp", "serve"}
	if env != "" {
		args = append(args, "--env", env)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if readOnly {
		args = append(args, "--read-only")
	}
	if noBillable {
		args = append(args, "--no-billable")
	}
	return mcpLaunch{Bin: "dna", Args: args}
}
