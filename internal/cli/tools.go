package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/openapi"
	"github.com/domainreseller/cli/internal/ops"
	"github.com/domainreseller/cli/internal/output"
	"github.com/domainreseller/cli/internal/ui"
)

// ---------------------------------------------------------------- api

func (a *App) newAPICmd() *cobra.Command {
	var fields []string
	var body string
	var method string
	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Send a raw request to any API endpoint",
		Long: `Sends an authenticated request to any endpoint and prints the response, like
"gh api". Use it for endpoints or parameters the typed commands do not cover.

The method defaults to GET, or POST when a body is given. Query parameters
come from -f key=value (repeatable); the body from --body (JSON, @file or -).
Read-only mode blocks every method except GET. Raw calls are not checked for
billable effects - prefer the typed commands for registrations and renewals.`,
		Example: `  dna api /api/v1/domains/info -f DomainName=example.com
  dna api -X POST /api/v1/domains/search --body '{"domainName":"example.com"}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			if !strings.HasPrefix(path, "/api/") {
				path = "/api/v1" + path
			}
			q := url.Values{}
			if i := strings.Index(path, "?"); i >= 0 {
				parsed, err := url.ParseQuery(path[i+1:])
				if err != nil {
					return usageErr(err)
				}
				q, path = parsed, path[:i]
			}
			for _, f := range fields {
				k, v, ok := strings.Cut(f, "=")
				if !ok {
					return usagef("-f %q: expected key=value", f)
				}
				q.Add(k, v)
			}
			m := strings.ToUpper(method)
			req := &dna.Request{Method: m, Path: path, Query: q}
			if body != "" {
				v, err := a.readJSONArg("--body", body)
				if err != nil {
					return err
				}
				req.Body = v
				if m == "" {
					req.Method = http.MethodPost
				}
			}
			if req.Method == "" {
				req.Method = http.MethodGet
			}
			r, err := a.resolved()
			if err != nil {
				return err
			}
			if r.ReadOnly && req.Method != http.MethodGet {
				return &PolicyError{Operation: req.Method + " " + path, Source: r.ReadOnlySource}
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if a.flags.dryRun {
				op := &ops.Operation{ID: "api", Safety: ops.Write}
				return a.printDryRun(cmd.Context(), client, op, req, r)
			}
			var raw json.RawMessage
			if _, err := client.Do(cmd.Context(), req, &raw); err != nil {
				return err
			}
			return a.render(raw, output.Options{})
		},
	}
	cmd.Flags().StringVarP(&method, "method", "X", "", "HTTP method (default GET, or POST with --body)")
	cmd.Flags().StringArrayVarP(&fields, "field", "f", nil, "Query parameter key=value (repeatable)")
	cmd.Flags().StringVarP(&body, "body", "d", "", "JSON body: inline, @file or - (stdin)")
	return cmd
}

// ---------------------------------------------------------------- schema

// opSummary is the machine-readable description of an operation.
type opSummary struct {
	ID          string   `json:"id"`
	Command     string   `json:"command"`
	Tool        string   `json:"mcpTool"`
	Summary     string   `json:"summary"`
	Description string   `json:"description,omitempty"`
	Safety      string   `json:"safety"`
	Confirm     bool     `json:"requiresConfirmation"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Paged       bool     `json:"paged,omitempty"`
	Args        []string `json:"args,omitempty"`
	Returns     string   `json:"returns,omitempty"`
}

func summarizeOp(op *ops.Operation) opSummary {
	cmd := "dna " + strings.Join(op.CommandPath(), " ")
	var args []string
	for _, a := range op.Args {
		args = append(args, strings.TrimSuffix(a, "..."))
	}
	return opSummary{
		ID: op.ID, Command: cmd, Tool: op.ToolName(), Summary: op.Summary, Description: op.Description,
		Safety: string(op.Safety), Confirm: op.Safety.NeedsConfirmation(), Method: op.Method, Path: op.Path,
		Paged: op.Paged(), Args: args, Returns: op.ResultType(),
	}
}

func (a *App) newSchemaCmd() *cobra.Command {
	var withResult bool
	cmd := &cobra.Command{
		Use:   "schema [operation]",
		Short: "Describe operations as JSON (inputs as JSON Schema) for tools and AI agents",
		Long: `Without arguments, lists every operation with its command, MCP tool name,
safety class and HTTP route. With an operation (ID like "domain.register",
tool name like "domain_register" or command path like "domain register"),
prints its full input JSON Schema, an example input and, with --result, the
JSON Schema of the response.

The input accepted by --body and by the MCP tool is exactly this schema.`,
		Example: `  dna schema
  dna schema domain.register
  dna schema dns.add --result
  dna schema --jq '.[] | select(.safety=="billable") | .command'`,
		Args: cobra.MaximumNArgs(5),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				var list []opSummary
				for _, op := range ops.All() {
					s := summarizeOp(op)
					s.Description = ""
					list = append(list, s)
				}
				return a.printJSON(list)
			}
			op, ok := ops.Lookup(strings.Join(args, " "))
			if !ok {
				return usagef("unknown operation %q (run `dna schema` to list them)", strings.Join(args, " "))
			}
			out := map[string]any{
				"operation":   summarizeOp(op),
				"inputSchema": op.InputSchema(),
				"example":     op.ExampleInput(),
			}
			if withResult && op.Spec().Result != nil {
				d := openapi.MustLoad()
				out["resultSchema"] = d.ToJSONSchema(op.Spec().Result, func(owner, prop string) (string, []any) {
					if e, ok := dna.EnumFor(owner, prop); ok {
						return "Values: " + e.Summary() + ".", nil
					}
					return "", nil
				})
			}
			return a.printJSON(out)
		},
	}
	cmd.Flags().BoolVar(&withResult, "result", false, "Include the response JSON Schema")
	return cmd
}

// ---------------------------------------------------------------- enums

func (a *App) newEnumsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enums [name]",
		Short: "List the meaning of the API's coded values (statuses, types, ...)",
		Long:  "The API returns many fields as numbers (status 0, type 2, ...). This lists every enum with its values and meanings. Commands accept names wherever an enum is expected (e.g. --dnssec-mode DisableDnssec).",
		Example: `  dna enums
  dna enums DomainStatus
  dna enums RecordType -o json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			list := dna.Enums()
			if len(args) == 1 {
				e, ok := dna.LookupEnum(args[0])
				if !ok {
					names := make([]string, len(list))
					for i, e := range list {
						names[i] = e.Name
					}
					return usagef("unknown enum %q (known: %s)", args[0], strings.Join(names, ", "))
				}
				list = []*dna.EnumInfo{e}
			}
			if a.machineOutput() {
				if len(args) == 1 {
					return a.printJSON(list[0])
				}
				return a.printJSON(list)
			}
			p := a.outP()
			for i, e := range list {
				if i > 0 {
					fmt.Fprintln(a.Out)
				}
				fmt.Fprintf(a.Out, "%s %s %s\n", p.BoldFg(ui.Accent, e.Name), p.Muted("("+e.Kind+")"), p.Muted("· "+e.Description))
				for _, v := range e.Values {
					desc := ""
					if v.Description != "" {
						desc = "  " + p.Muted(v.Description)
					}
					if e.Kind == "string" {
						fmt.Fprintf(a.Out, "  %s%s\n", ui.PadRight(v.Name, 28), desc)
					} else {
						fmt.Fprintf(a.Out, "  %s  %s%s\n", p.Fg(ui.Amber, fmt.Sprintf("%4v", v.Value)), ui.PadRight(v.Name, 28), desc)
					}
				}
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------- doctor

func (a *App) newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose configuration, connectivity and credentials",
		Long:  "Runs checks an operator or AI agent can use to fix setup problems: config file, credential sources, network reachability of the selected environment and a credential check. Exits non-zero when a check fails.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			type check struct {
				Name   string `json:"name"`
				OK     bool   `json:"ok"`
				Detail string `json:"detail"`
				Hint   string `json:"hint,omitempty"`
			}
			var checks []check
			var firstErr error
			add := func(name string, err error, detail, hint string) {
				c := check{Name: name, OK: err == nil, Detail: detail}
				if err != nil {
					c.Detail = err.Error()
					c.Hint = hint
					if firstErr == nil {
						firstErr = err
					}
				}
				checks = append(checks, c)
			}

			cfg, err := a.config()
			add("config", err, "config file: "+a.configPath()+fmt.Sprintf(" (%d profiles)", len(profilesOf(cfg))), "Fix or delete the config file.")
			r, err := a.resolved()
			if err != nil {
				add("settings", err, "", "")
				return a.finishDoctor(checks, firstErr)
			}
			add("environment", nil, fmt.Sprintf("%s via %s -> %s", r.Environment, r.EnvSource, r.BaseURL), "")
			if r.HasCredentials() {
				add("credentials", nil, fmt.Sprintf("reseller ID from %s, API key from %s", r.ResellerIDSource, r.APIKeySource), "")
			} else {
				add("credentials", dna.ErrNoCredentials, "", "Run `dna auth login` (add --env ote for the test environment).")
			}

			start := time.Now()
			hc := a.HTTPClient
			if hc == nil {
				hc = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			}
			req, _ := http.NewRequestWithContext(cmd.Context(), http.MethodGet, strings.TrimRight(r.BaseURL, "/")+"/swagger/v1/swagger.json", nil)
			req.Header.Set("User-Agent", a.userAgent())
			resp, err := hc.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode != 200 {
					err = fmt.Errorf("unexpected HTTP %d from %s", resp.StatusCode, req.URL)
				}
			}
			add("network", err, fmt.Sprintf("%s reachable in %s", r.BaseURL, time.Since(start).Round(time.Millisecond)), "Check internet access, proxies (HTTPS_PROXY) and firewalls.")

			if r.HasCredentials() {
				acct, err := a.verifyCredentials(cmd.Context())
				detail := ""
				if acct != nil {
					detail = fmt.Sprintf("authenticated as %s (%.2f USD / %.2f TRY)", acct.ResellerName, acct.UsdBalance, acct.TryBalance)
				}
				hint := ""
				if ae, ok := err.(*dna.APIError); ok {
					hint = ae.Hint()
				}
				add("authentication", err, detail, hint)
			}
			if r.ReadOnly {
				add("read-only", nil, "read-only mode is ON ("+r.ReadOnlySource+"): changes are blocked", "")
			}
			return a.finishDoctor(checks, firstErr)
		},
	}
}

func profilesOf(cfg interface{ ProfileNames() []string }) []string {
	if cfg == nil {
		return nil
	}
	return cfg.ProfileNames()
}

func (a *App) finishDoctor(checks any, firstErr error) error {
	if a.machineOutput() {
		_ = a.printJSON(map[string]any{"ok": firstErr == nil, "checks": checks})
		return firstErr
	}
	b, _ := json.Marshal(checks)
	var list []map[string]any
	_ = json.Unmarshal(b, &list)
	p := a.outP()
	var sb strings.Builder
	sb.WriteString("\n")
	passed, failed := 0, 0
	for _, c := range list {
		mark := p.BoldFg(ui.Green, "✓")
		if ok, _ := c["ok"].(bool); !ok {
			mark = p.BoldFg(ui.Red, "✗")
			failed++
		} else {
			passed++
		}
		sb.WriteString(fmt.Sprintf("  %s %s %v\n", mark, p.Bold(ui.PadRight(fmt.Sprint(c["name"]), 15)), c["detail"]))
		if h, _ := c["hint"].(string); h != "" {
			sb.WriteString("    " + strings.Repeat(" ", 15) + p.Hint(styleCode(p, h)) + "\n")
		}
	}
	summary := p.Fg(ui.Green, fmt.Sprintf("%d passed", passed))
	if failed > 0 {
		summary += p.Muted(" · ") + p.Fg(ui.Red, fmt.Sprintf("%d failed", failed))
	}
	sb.WriteString("\n  " + summary + "\n\n")
	fmt.Fprint(a.Out, sb.String())
	return firstErr
}

// ---------------------------------------------------------------- version

func (a *App) newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			info := map[string]any{
				"version": a.Build.Version, "commit": a.Build.Commit, "date": a.Build.Date,
				"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
				"apiSpec": openapi.MustLoad().Info.Title + " " + openapi.MustLoad().Info.Version,
			}
			if a.machineOutput() {
				return a.printJSON(info)
			}
			p := a.outP()
			fmt.Fprintf(a.Out, "%s %s %s\n", p.BoldFg(ui.Accent, "dna"), p.Bold(a.Build.Version), p.Muted(fmt.Sprintf("(commit %s, built %s)", orDash(a.Build.Commit), orDash(a.Build.Date))))
			fmt.Fprintln(a.Out, p.Muted(fmt.Sprintf("%s %s/%s · API %s", runtime.Version(), runtime.GOOS, runtime.GOARCH, info["apiSpec"])))
			return nil
		},
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ResolveBuildInfo fills empty fields from the Go module build info, so
// `go install` builds report a meaningful version.
func ResolveBuildInfo(b BuildInfo) BuildInfo {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if b.Version == "" || b.Version == "dev" {
			if v := bi.Main.Version; v != "" && v != "(devel)" {
				b.Version = strings.TrimPrefix(v, "v")
			}
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if b.Commit == "" && len(s.Value) >= 7 {
					b.Commit = s.Value[:7]
				}
			case "vcs.time":
				if b.Date == "" {
					b.Date = s.Value
				}
			}
		}
	}
	if b.Version == "" {
		b.Version = "dev"
	}
	return b
}

// sortedGroupNames returns group names in a stable order.
func sortedGroupNames() []string {
	var names []string
	for k := range ops.Groups() {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
