// Package cli implements the dna command line interface.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/domainreseller/cli/internal/config"
	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/output"
	"github.com/domainreseller/cli/internal/ui"
)

// BuildInfo identifies the binary.
type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
}

// App holds process-wide state: IO streams, environment and global flags.
// Tests construct it directly with buffers and a fake environment.
type App struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer

	Getenv func(string) string
	Build  BuildInfo

	// Terminal detection; overridable in tests.
	StdinTTY, StdoutTTY, StderrTTY bool
	// NoVT is set when the console cannot show ANSI escape sequences
	// (Windows versions before 10); colors and the spinner stay off then.
	NoVT bool
	// Width is the terminal width; 0 detects it from stdout.
	Width int

	// inShell marks an App the interactive shell runs one command with.
	inShell bool

	// HTTPClient, when set, is used for API calls (tests).
	HTTPClient *http.Client

	flags globalFlags
	cfg   *config.File
	res   *config.Resolved
	inR   *bufio.Reader // see stdin()
}

type globalFlags struct {
	output     string
	jq         string
	env        string
	ote        bool
	profile    string
	resellerID string
	apiKey     string
	baseURL    string
	configPath string
	yes        bool
	dryRun     bool
	readOnly   bool
	verbose    bool
	noColor    bool
	timeout    time.Duration
}

// NewApp returns an App wired to the real process.
func NewApp(build BuildInfo) *App {
	a := &App{
		In:        os.Stdin,
		Out:       os.Stdout,
		Err:       os.Stderr,
		Getenv:    os.Getenv,
		Build:     build,
		StdinTTY:  term.IsTerminal(int(os.Stdin.Fd())),
		StdoutTTY: term.IsTerminal(int(os.Stdout.Fd())),
		StderrTTY: term.IsTerminal(int(os.Stderr.Fd())),
	}
	if (a.StdoutTTY && !ui.EnableVT(os.Stdout)) || (a.StderrTTY && !ui.EnableVT(os.Stderr)) {
		a.NoVT = true
	}
	return a
}

// Run executes the CLI with args and returns the process exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	root := a.newRootCmd()
	root.SetArgs(args)
	root.SetIn(a.In)
	root.SetOut(a.Out)
	root.SetErr(a.Err)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	return a.reportError(err)
}

// interactive reports whether we may prompt the user. Never under an AI
// agent: a prompt would block it, so it gets exit code 9 instead.
func (a *App) interactive() bool { return a.StdinTTY && a.StderrTTY && !a.agentMode() }

// agentEnvVars are set by AI coding agents in the shells they run commands
// in. AI_AGENT is the generic convention; the others are tool specific.
var agentEnvVars = []string{"AI_AGENT", "CLAUDECODE", "GEMINI_CLI", "CODEX_SANDBOX", "CURSOR_AGENT"}

// agentMode reports whether an AI agent runs this command. Agents sometimes
// run commands in a pseudo-terminal, which looks like a person at a
// terminal; in agent mode dna still behaves as on a pipe: JSON output, no
// prompts, no spinner. DNA_AGENT=1 forces it on, DNA_AGENT=0 off.
func (a *App) agentMode() bool {
	switch strings.ToLower(strings.TrimSpace(a.Getenv(config.EnvAgent))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	for _, k := range agentEnvVars {
		if a.Getenv(k) != "" {
			return true
		}
	}
	return false
}

// human reports whether a person is watching the terminal (stdout).
func (a *App) human() bool { return a.StdoutTTY && !a.agentMode() }

// painter returns the color painter for a stream.
func (a *App) painter(tty bool) ui.Painter {
	if a.flags.noColor || a.NoVT {
		return ui.Painter{}
	}
	return ui.Painter{Level: ui.DetectLevel(tty, a.Getenv)}
}

// outP and errP paint stdout and stderr.
func (a *App) outP() ui.Painter { return a.painter(a.StdoutTTY) }
func (a *App) errP() ui.Painter { return a.painter(a.StderrTTY) }

// width is the terminal width for tables (0 when stdout is not a terminal).
func (a *App) width() int {
	if a.Width > 0 {
		return a.Width
	}
	if f, ok := a.Out.(*os.File); ok && a.StdoutTTY {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			return w
		}
	}
	return 0
}

// spin shows a spinner on stderr while a request runs, when a person is
// watching. It returns functions that relabel and stop (erase) it.
func (a *App) spin(label string) (update func(string), stop func()) {
	if !a.StderrTTY || a.inShell || a.agentMode() || a.flags.verbose || a.NoVT || a.Getenv("TERM") == "dumb" {
		return func(string) {}, func() {}
	}
	s := ui.NewSpinner(a.Err, a.errP())
	s.MaxWidth = a.width()
	s.Start(label)
	return s.Update, s.Stop
}

// format resolves --output (auto -> table on a terminal, json otherwise).
func (a *App) format() (output.Format, error) {
	s := a.flags.output
	if s == "" {
		s = a.Getenv(config.EnvOutput)
	}
	f, err := output.ParseFormat(s)
	if err != nil {
		return "", usageErr(err)
	}
	if f == output.Auto {
		if a.human() {
			return output.Table, nil
		}
		return output.JSON, nil
	}
	return f, nil
}

// machineOutput reports whether errors should be printed as JSON.
func (a *App) machineOutput() bool {
	f, err := a.format()
	return err == nil && f != output.Table
}

func (a *App) configPath() string {
	if a.flags.configPath != "" {
		return a.flags.configPath
	}
	return config.DefaultPath(a.Getenv)
}

func (a *App) config() (*config.File, error) {
	if a.cfg != nil {
		return a.cfg, nil
	}
	f, err := config.Load(a.configPath())
	if err != nil {
		return nil, err
	}
	a.cfg = f
	return f, nil
}

func (a *App) resolved() (*config.Resolved, error) {
	if a.res != nil {
		return a.res, nil
	}
	f, err := a.config()
	if err != nil {
		return nil, err
	}
	env := a.flags.env
	if a.flags.ote {
		if env != "" && !strings.EqualFold(env, "ote") {
			return nil, usageErr(errors.New("--ote conflicts with --env " + env))
		}
		env = "ote"
	}
	r, err := config.Resolve(f, config.Overrides{
		Profile: a.flags.profile, Env: env, ResellerID: a.flags.resellerID,
		APIKey: a.flags.apiKey, BaseURL: a.flags.baseURL, ReadOnly: a.flags.readOnly,
	}, a.Getenv)
	if err != nil {
		return nil, usageErr(err)
	}
	if a.flags.ote {
		r.EnvSource = strings.Replace(r.EnvSource, "flag --env", "flag --ote", 1)
		r.ProfileSource = strings.Replace(r.ProfileSource, "flag --env", "flag --ote", 1)
	}
	for _, w := range r.Warnings {
		a.warn(w)
	}
	a.res = r
	return r, nil
}

func (a *App) userAgent() string {
	return "DomainNameAPI-CLI/" + a.Build.Version + " (+https://github.com/domainreseller/cli)"
}

// client builds an API client from the resolved configuration.
func (a *App) client() (*dna.Client, error) {
	r, err := a.resolved()
	if err != nil {
		return nil, err
	}
	opts := []dna.Option{dna.WithBaseURL(r.BaseURL), dna.WithUserAgent(a.userAgent())}
	if a.HTTPClient != nil {
		opts = append(opts, dna.WithHTTPClient(a.HTTPClient))
	}
	if a.flags.timeout > 0 {
		opts = append(opts, dna.WithTimeout(a.flags.timeout))
	}
	if a.flags.verbose {
		opts = append(opts, dna.WithTrace(a.trace))
	}
	return dna.NewClient(r.ResellerID, r.APIKey, opts...)
}

func (a *App) trace(ev dna.TraceEvent) {
	switch ev.Kind {
	case "request":
		fmt.Fprintf(a.Err, "→ %s %s (attempt %d)\n", ev.Method, ev.URL, ev.Attempt)
		if len(ev.Body) > 0 {
			fmt.Fprintf(a.Err, "  %s\n", ev.Body)
		}
	case "response":
		if ev.Err != nil {
			fmt.Fprintf(a.Err, "← error after %s: %v\n", ev.Duration.Round(time.Millisecond), ev.Err)
			return
		}
		fmt.Fprintf(a.Err, "← %d in %s", ev.Status, ev.Duration.Round(time.Millisecond))
		if q := ev.Quota; q != nil {
			fmt.Fprintf(a.Err, " (quota %s: used %.0f/%.0f, cost %.0f, delayed %.1fs)", q.Scope, q.Used, q.Limit, q.Cost, q.Delay)
		}
		fmt.Fprintln(a.Err)
	case "retry":
		fmt.Fprintf(a.Err, "↻ retrying in %s (status %d)\n", ev.Wait.Round(time.Millisecond), ev.Status)
	}
}

func (a *App) warn(msg string) {
	if a.machineOutput() {
		b, _ := json.Marshal(map[string]string{"warning": msg})
		fmt.Fprintln(a.Err, string(b))
		return
	}
	fmt.Fprintln(a.Err, a.errP().BoldFg(ui.Amber, "▲ warning")+" "+msg)
}

// render prints an API response body using the global output flags.
func (a *App) render(data []byte, opts output.Options) error {
	f, err := a.format()
	if err != nil {
		return err
	}
	opts.Format = f
	opts.JQ = a.flags.jq
	opts.Pretty = a.StdoutTTY
	switch {
	case f == output.Table:
		opts.Painter, opts.Width = a.outP(), a.width()
	case a.human():
		// JSON on a terminal gets syntax colors; on a pipe (scripts,
		// agents) it never does, whatever FORCE_COLOR says.
		opts.Painter = a.outP()
	}
	return output.Render(a.Out, data, opts)
}

// printJSON prints a value (not an API response) honouring --jq/--output.
// Without --jq the encoding keeps struct and ordered-map key order.
func (a *App) printJSON(v any) error {
	f, err := a.format()
	if err != nil {
		return err
	}
	if a.flags.jq == "" && f != output.JSONL {
		p := ui.Painter{}
		if a.human() {
			p = a.outP()
		}
		return output.WriteJSONColor(a.Out, v, a.StdoutTTY, p)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if f == output.Table || f == output.Raw {
		f = output.JSON // structured CLI data has no table form
	}
	opts := output.Options{Format: f, JQ: a.flags.jq, Pretty: a.StdoutTTY}
	if a.human() {
		opts.Painter = a.outP()
	}
	return output.Render(a.Out, b, opts)
}

func addGlobalFlags(cmd *cobra.Command, f *globalFlags) {
	pf := cmd.PersistentFlags()
	pf.StringVarP(&f.output, "output", "o", "", "Output format: auto, json, jsonl, table, raw (env DNA_OUTPUT; auto = table on a terminal, json otherwise)")
	pf.StringVarP(&f.jq, "jq", "q", "", "Filter the JSON result with a jq expression (strings are printed raw)")
	pf.StringVar(&f.env, "env", "", "API environment: production or ote (env DNA_ENV)")
	pf.BoolVar(&f.ote, "ote", false, "Shorthand for --env ote (the test environment)")
	pf.StringVarP(&f.profile, "profile", "p", "", "Config profile to use (env DNA_PROFILE)")
	pf.StringVar(&f.resellerID, "reseller-id", "", "Reseller ID (UUID); prefer DNA_RESELLER_ID or 'dna auth login'")
	pf.StringVar(&f.apiKey, "api-key", "", "API key; prefer DNA_API_KEY or 'dna auth login' (flags end up in shell history)")
	pf.StringVar(&f.baseURL, "base-url", "", "Override the API base URL (env DNA_BASE_URL)")
	pf.StringVar(&f.configPath, "config", "", "Config file path (env DNA_CONFIG)")
	pf.BoolVarP(&f.yes, "yes", "y", false, "Confirm billable/destructive operations without prompting")
	pf.BoolVar(&f.dryRun, "dry-run", false, "Validate and print the HTTP request instead of sending it")
	pf.BoolVar(&f.readOnly, "read-only", false, "Refuse every operation that changes something (env DNA_READ_ONLY=1)")
	pf.BoolVarP(&f.verbose, "verbose", "v", false, "Log HTTP requests, responses, retries and quota to stderr")
	pf.BoolVar(&f.noColor, "no-color", false, "Disable colors (env NO_COLOR)")
	pf.DurationVar(&f.timeout, "timeout", 60*time.Second, "Per-request timeout")
	_ = pf.MarkHidden("base-url")
}
