package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ops"
	"github.com/domainreseller/cli/internal/ui"
)

// child returns an App that runs one command for the shell: human output
// rendered for the terminal into a buffer, no prompts (the shell asks for
// confirmation itself) and no spinner (the shell shows its own).
func (a *App) child(out, errw io.Writer, width int) *App {
	if out == nil {
		out = io.Discard
	}
	if errw == nil {
		errw = io.Discard
	}
	return &App{
		In: strings.NewReader(""), Out: out, Err: errw, Getenv: a.Getenv, Build: a.Build,
		StdinTTY: false, StdoutTTY: true, StderrTTY: true, NoVT: a.NoVT, Width: width,
		HTTPClient: a.HTTPClient, flags: a.flags, inShell: true,
	}
}

// args builds the full argument list of a shell command: global flags given
// to `dna`, the environment chosen with /env, then the command itself (later
// flags win, so a command's own --env still applies).
func (s *shell) args(cmd []string) []string {
	out := append([]string{}, s.base...)
	if s.env != "" {
		out = append(out, "--env", s.env)
	}
	return append(out, cmd...)
}

// ---------------------------------------------------------------- submit

var domainish = regexp.MustCompile(`^(?i)[a-z0-9\p{L}][a-z0-9\p{L}-]*(\.[a-z0-9\p{L}-]+)*\.?$`)

func (s *shell) submit(line string) tea.Cmd {
	if line == "" {
		return nil
	}
	s.remember(line)
	echo := tea.Println("\n" + s.p.BoldFg(ui.Green, "›") + " " + s.p.Bold(line))
	if strings.HasPrefix(line, "/") {
		return tea.Sequence(echo, s.slash(line))
	}
	words, err := splitWords(line)
	if err != nil {
		return tea.Sequence(echo, tea.Println(s.p.Fg(ui.Red, "  ✗ ")+err.Error()))
	}
	if len(words) > 0 && words[0] == "dna" {
		words = words[1:]
	}
	if len(words) == 0 {
		return tea.Sequence(echo, tea.Println(s.helpText()))
	}
	switch strings.ToLower(words[0]) {
	case "exit", "quit", "q":
		return tea.Quit
	case "help", "?":
		if len(words) == 1 {
			return tea.Sequence(echo, tea.Println(s.helpText()))
		}
	case "clear", "cls":
		return tea.Sequence(tea.ClearScreen, tea.Println(s.welcome()))
	}
	if s.isCommand(words[0]) {
		return tea.Sequence(echo, s.runCommand(words))
	}
	if len(words) == 1 && domainish.MatchString(words[0]) {
		return tea.Sequence(echo, s.runCommand(searchArgs(words[0])))
	}
	return tea.Sequence(echo, tea.Println("  "+s.p.Fg(ui.Amber, "▲ ")+"Not a command or a domain name. Press "+s.p.Command("↓")+" on an empty line for quick actions, "+s.p.Command("/")+" for every command."))
}

// searchArgs checks a typed name: an exact domain, or a label across the
// popular extensions.
func searchArgs(q string) []string {
	q = strings.ToLower(strings.TrimSuffix(q, "."))
	if strings.Contains(q, ".") {
		return []string{"domain", "search", q}
	}
	args := []string{"domain", "bulk-search"}
	for _, tld := range popularTLDs {
		args = append(args, q+"."+tld)
	}
	return args
}

func (s *shell) isCommand(word string) bool {
	for _, c := range s.root.Commands() {
		if c.Name() == word || c.HasAlias(word) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- slash commands

type slashCommand struct {
	name, args, desc string
	run              func(s *shell, arg string) tea.Cmd
}

var slashCommands []slashCommand

func init() {
	// withDomain runs f with the given domain, or lets the user pick one.
	withDomain := func(title string, f func(s *shell, d string) tea.Cmd) func(s *shell, arg string) tea.Cmd {
		return func(s *shell, arg string) tea.Cmd {
			if arg != "" {
				return f(s, arg)
			}
			return s.pickDomain(title, f, false)
		}
	}
	slashCommands = []slashCommand{
		{"/domains", "", "Your domains: search, then manage one", func(s *shell, _ string) tea.Cmd { return s.openBrowser() }},
		{"/manage", "<domain>", "Everything you can do with one domain", withDomain("Manage…", func(s *shell, d string) tea.Cmd { s.push(s.domainMenu(d)); return s.ensureDomains() })},
		{"/register", "", "Check a domain, see the price, register it", func(s *shell, _ string) tea.Cmd { return s.registerStart() }},
		{"/search", "<name>", "Availability and price across popular extensions", func(s *shell, a string) tea.Cmd {
			if a == "" {
				s.notice = "Just type a name at the prompt: results appear while you type."
				return nil
			}
			return s.runCommand(searchArgs(a))
		}},
		{"/info", "<domain>", "Details of a domain", withDomain("Details of…", func(s *shell, d string) tea.Cmd {
			return s.start([]string{"domain", "info", d}, "Reading "+d, runOpts{})
		})},
		{"/dns", "<domain>", "DNS records of a domain", withDomain("DNS records of…", func(s *shell, d string) tea.Cmd {
			return s.start([]string{"dns", "list", d}, "Reading the zone of "+d, runOpts{})
		})},
		{"/renew", "<domain>", "Renew a domain (asks for the years, then confirms)", withDomain("Renew…", func(s *shell, d string) tea.Cmd { s.push(s.renewForm(d)); return nil })},
		{"/contact", "", "Create a contact handle in a short form (saved for registrations)", func(s *shell, _ string) tea.Cmd { s.push(s.contactForm(nil, nil)); return nil }},
		{"/contacts", "", "The contact handles dna knows, the default first", func(s *shell, _ string) tea.Cmd {
			return s.start([]string{"contact", "list"}, "Reading your contacts", runOpts{})
		}},
		{"/balance", "", "Balance and reseller account", func(s *shell, _ string) tea.Cmd {
			return s.start([]string{"account", "get"}, "Counting your balance", runOpts{})
		}},
		{"/transactions", "", "Latest balance transactions", func(s *shell, _ string) tea.Cmd {
			return s.start([]string{"account", "transactions", "--max-result-count", "15"}, "Reading transactions", runOpts{})
		}},
		{"/env", "ote|production", "Switch the environment for this session", func(s *shell, a string) tea.Cmd { return s.switchEnv(a) }},
		{"/login", "", "Log in to the current environment", func(s *shell, _ string) tea.Cmd { return s.execInteractive("auth", "login") }},
		{"/status", "", "Which credentials are in use", func(s *shell, _ string) tea.Cmd {
			return s.start([]string{"auth", "status"}, "Checking the credentials", runOpts{})
		}},
		{"/doctor", "", "Check config, network and credentials", func(s *shell, _ string) tea.Cmd {
			return s.start([]string{"doctor"}, "Running checks", runOpts{})
		}},
		{"/agent", "", "Connect Claude Code, ChatGPT, Cursor or another AI agent", func(s *shell, _ string) tea.Cmd { return s.agentPicker() }},
		{"/mcp", "", "Same as /agent: connect an AI agent through MCP", func(s *shell, _ string) tea.Cmd { return s.agentPicker() }},
		{"/help", "", "Keys and examples", func(s *shell, _ string) tea.Cmd { return tea.Println(s.helpText()) }},
		{"/clear", "", "Clear the screen", func(s *shell, _ string) tea.Cmd { return tea.Sequence(tea.ClearScreen, tea.Println(s.welcome())) }},
		{"/exit", "", "Leave the shell", func(*shell, string) tea.Cmd { return tea.Quit }},
	}
}

func (s *shell) slash(line string) tea.Cmd {
	name, arg, _ := strings.Cut(strings.TrimSpace(line), " ")
	arg = strings.TrimSpace(arg)
	for _, c := range slashCommands {
		if c.name == name {
			return c.run(s, arg)
		}
	}
	return tea.Println("  " + s.p.Fg(ui.Amber, "▲ ") + "Unknown command " + s.p.Bold(name) + ". Type " + s.p.Command("/") + " to see the menu.")
}

func (s *shell) switchEnv(arg string) tea.Cmd {
	p := s.p
	switch strings.ToLower(arg) {
	case "ote", "test":
		s.env = "ote"
	case "production", "prod", "live":
		s.env = "production"
	case "":
		return tea.Println("  Environment: " + p.Bold(s.currentEnv()) + p.Muted(". Switch with ") + p.Command("/env ote") + p.Muted(" or ") + p.Command("/env production"))
	default:
		return tea.Println("  " + p.Fg(ui.Amber, "▲ ") + "Use " + p.Command("/env ote") + " or " + p.Command("/env production"))
	}
	s.status.loaded = false
	s.live = liveState{seq: s.live.seq + 1}
	s.domains = domainCache{}
	msg := "  " + p.Badge(ui.Green, "OTE") + " Switched to the test environment. Nothing here costs real money."
	if s.env == "production" {
		msg = "  " + p.Badge(ui.Red, "PRODUCTION") + " Switched to production: billable steps charge your real balance (each one asks first)."
	}
	return tea.Batch(tea.Println(msg), s.fetchStatus())
}

// execInteractive hands the terminal to a dna command that asks questions
// (such as auth login) and returns to the shell afterwards.
type execDoneMsg struct {
	err  error
	note string // printed when the program ends
}

func (s *shell) execInteractive(cmd ...string) tea.Cmd {
	exe, err := os.Executable()
	if err != nil {
		return tea.Println("  " + s.p.Fg(ui.Red, "✗ ") + err.Error())
	}
	c := exec.Command(exe, s.args(cmd)...)
	return tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{err: err} })
}

// ---------------------------------------------------------------- running commands

// verbs are the spinner labels while a typed command runs.
var verbs = map[string]string{
	"domain search": "Asking the registry", "domain bulk-search": "Checking availability",
	"domain list": "Fetching your domains", "domain info": "Reading the domain record",
	"dns": "Reading the zone", "account": "Counting your balance", "doctor": "Running checks",
	"domain renew": "Renewing", "domain register": "Registering", "domain transfer": "Starting the transfer",
	"ssl": "Talking to the certificate authority", "dnssec": "Checking DNSSEC", "contact": "Looking up contacts",
}

func verbFor(words []string) string {
	if len(words) >= 2 {
		if v, ok := verbs[words[0]+" "+words[1]]; ok {
			return v
		}
	}
	if len(words) >= 1 {
		if v, ok := verbs[words[0]]; ok {
			return v
		}
	}
	return "Talking to Domain Name API"
}

// lookup finds the cobra command of words and its operation, if any.
func (s *shell) lookup(words []string) (*cobra.Command, *ops.Operation) {
	c, _, err := s.root.Find(words)
	if err != nil || c == nil {
		return nil, nil
	}
	if id := c.Annotations["operation"]; id != "" {
		op, _ := ops.Lookup(id)
		return c, op
	}
	return c, nil
}

func hasFlag(words []string, names ...string) bool {
	for _, w := range words {
		for _, n := range names {
			if w == n || strings.HasPrefix(w, n+"=") {
				return true
			}
		}
	}
	return false
}

// runOpts tune how a finished command is reported.
type runOpts struct {
	onDone   func(s *shell, code int) tea.Cmd
	success  string              // printed (with ✓) instead of the output on success
	decorate func(string) string // rewrites the output on success
}

type runDoneMsg struct {
	out    string
	code   int
	billed bool
	opts   runOpts
}

// runCommand runs a dna command and prints its output above the prompt.
// Billable and destructive commands first show a confirmation box built
// from a dry run of the exact request.
func (s *shell) runCommand(words []string) tea.Cmd {
	if len(words) >= 2 && words[0] == "auth" && words[1] == "login" {
		return s.execInteractive(words...)
	}
	_, op := s.lookup(words)
	if op != nil && op.Safety.NeedsConfirmation() && !hasFlag(words, "--yes", "-y", "--dry-run") {
		return s.startDryRun(words)
	}
	return s.start(words, verbFor(words), runOpts{})
}

func (s *shell) start(words []string, label string, o runOpts) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	s.run = runState{active: true, label: label, start: time.Now(), cancel: cancel}
	app, args, width := s.app, s.args(words), s.width
	_, op := s.lookup(words)
	billed := op != nil && op.Safety == ops.Billable
	return tea.Batch(s.tick(), func() tea.Msg {
		defer cancel()
		var out bytes.Buffer
		code := app.child(&out, &out, width).Run(ctx, args)
		return runDoneMsg{out: out.String(), code: code, billed: billed, opts: o}
	})
}

func (s *shell) onRunDone(m runDoneMsg) tea.Cmd {
	s.run = runState{}
	var cmds []tea.Cmd
	out := strings.TrimRight(m.out, "\n")
	switch {
	case m.code == 0 && m.opts.success != "":
		out = "  " + s.p.BoldFg(ui.Green, "✓") + " " + m.opts.success
	case m.code == 0 && m.opts.decorate != nil:
		out = m.opts.decorate(out)
	}
	if strings.TrimSpace(out) != "" {
		cmds = append(cmds, tea.Println(out))
	}
	if m.opts.onDone != nil {
		if c := m.opts.onDone(s, m.code); c != nil {
			cmds = append(cmds, c)
		}
	}
	if m.billed && m.code == 0 {
		cmds = append(cmds, s.fetchStatus(), invalidateAfter(s, 0))
	}
	return tea.Sequence(cmds...)
}

// fetchStatus loads the account for the status line.
func (s *shell) fetchStatus() tea.Cmd {
	c := s.app.child(nil, nil, 0)
	if s.env != "" {
		c.flags.env, c.flags.ote = s.env, false
	}
	return func() tea.Msg {
		r, err := c.resolved()
		if err != nil {
			return acctMsg{err: err}
		}
		st := acctMsg{env: string(r.Environment), profile: r.Profile, readOnly: r.ReadOnly}
		if !r.HasCredentials() {
			st.err = dna.ErrNoCredentials
			return st
		}
		client, err := c.client()
		if err != nil {
			st.err = err
			return st
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		st.acct, st.err = fetchAccount(ctx, client)
		return st
	}
}

// ---------------------------------------------------------------- confirmation

type dryRunMsg struct {
	words []string
	out   string
	code  int
	plan  map[string]any
}

// confirmBox asks before anything billable or destructive is sent. Cancel
// is selected by default.
type confirmBox struct {
	words   []string
	op      *ops.Operation
	plan    map[string]any
	yes     bool
	note    string
	actWord string
}

func (s *shell) startDryRun(words []string) tea.Cmd {
	app, args := s.app, s.args(append(append([]string{}, words...), "--dry-run", "-o", "json"))
	s.run = runState{active: true, label: "Preparing the request", start: time.Now(), cancel: func() {}}
	return tea.Batch(s.tick(), func() tea.Msg {
		var out, errb bytes.Buffer
		c := app.child(&out, &errb, 0)
		c.StdoutTTY, c.StderrTTY = false, true // JSON plan, human errors
		code := c.Run(context.Background(), args)
		m := dryRunMsg{words: words, code: code, out: errb.String()}
		if code == 0 {
			_ = json.Unmarshal(out.Bytes(), &m.plan)
		}
		return m
	})
}

func (s *shell) onDryRun(m dryRunMsg) tea.Cmd {
	s.run = runState{}
	note := s.pendingNote
	s.pendingNote = ""
	if m.code != 0 || m.plan == nil {
		return tea.Println(strings.TrimRight(m.out, "\n"))
	}
	_, op := s.lookup(m.words)
	act := "Confirm"
	if op != nil {
		parts := op.CommandPath()
		last := parts[len(parts)-1]
		act = strings.ToUpper(last[:1]) + last[1:]
	}
	s.push(&confirmBox{words: m.words, op: op, plan: m.plan, note: note, actWord: act})
	return nil
}

func (c *confirmBox) key(s *shell, k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "left", "right", "tab", "shift+tab", "h", "l":
		c.yes = !c.yes
		return nil
	case "y", "Y":
		c.yes = true
	case "n", "N", "esc", "q":
		c.yes = false
	case "enter":
	default:
		return nil
	}
	s.pop()
	if !c.yes {
		return tea.Println("  " + s.p.Muted("Canceled, nothing was sent."))
	}
	words := append(append([]string{}, c.words...), "--yes")
	return s.start(words, verbFor(c.words), runOpts{})
}

func (c *confirmBox) view(s *shell, w int) []string {
	p := s.p
	safety, _ := c.plan["safety"].(string)
	env, _ := c.plan["environment"].(string)
	req, _ := c.plan["request"].(map[string]any)
	border, badge, what := ui.Amber, p.Badge(ui.Amber, "BILLABLE"), "This charges your balance"
	if safety == string(ops.Destructive) {
		border, badge, what = ui.Red, p.Badge(ui.Red, "DESTRUCTIVE"), "This deletes data or is hard to undo"
	}
	title := "Confirm"
	if c.op != nil {
		title = c.op.Summary
	}
	lines := []string{p.Bold(title), ""}
	if body, ok := req["body"].(map[string]any); ok {
		keys := make([]string, 0, len(body))
		kw := 0
		for k := range body {
			keys = append(keys, k)
			if len(k) > kw {
				kw = len(k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := confirmValue(body[k])
			lines = append(lines, p.Muted(ui.PadRight(k, kw))+"  "+p.Bold(ui.Truncate(val, max(10, min(w, 90)-kw-10))))
		}
		lines = append(lines, "")
	}
	if c.note != "" {
		lines = append(lines, c.note, "")
	}
	method, _ := req["method"].(string)
	target, _ := req["url"].(string)
	if u, err := url.Parse(target); err == nil {
		target = u.Path
	}
	envText := p.BoldFg(ui.Green, "OTE") + p.Muted(" (test)")
	if !strings.EqualFold(env, "ote") {
		envText = p.BoldFg(ui.Red, "PRODUCTION")
	}
	lines = append(lines, what+" "+p.Muted("on")+" "+envText+".", p.Muted(method+" "+target))
	if !strings.EqualFold(env, "ote") && safety == string(ops.Billable) {
		lines = append(lines, p.Fg(ui.Red, "Real money: your balance is charged right away."))
	}
	var buttons string
	if c.yes {
		buttons = p.Muted("[ Cancel ]") + "   " + p.Badge(border, "› "+c.actWord)
	} else {
		buttons = p.Badge(ui.Navy, "› Cancel") + "   " + p.Muted("[ "+c.actWord+" ]")
	}
	lines = append(lines, "", buttons)
	box := p.Box(lines, min(w, 90), border, badge+" "+p.Muted("needs your OK"))
	return append(box, s.hint("←→ choose · enter confirm the choice · y "+strings.ToLower(c.actWord)+" · n / esc cancel"))
}

// confirmValue shows a request value the way a person reads it: lists
// joined with commas, contact handles by role, anything else as JSON.
func confirmValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var parts []string
		handles := map[string][]string{}
		var order []string
		for _, item := range x {
			switch it := item.(type) {
			case string:
				parts = append(parts, it)
			case map[string]any:
				h, _ := it["handle"].(string)
				role, _ := it["contactType"].(string)
				if h == "" {
					b, _ := json.Marshal(it)
					parts = append(parts, string(b))
					continue
				}
				if _, seen := handles[h]; !seen {
					order = append(order, h)
				}
				handles[h] = append(handles[h], role)
			}
		}
		for _, h := range order {
			if len(handles[h]) == 4 {
				parts = append(parts, h+" (all roles)")
			} else {
				parts = append(parts, h+" ("+strings.Join(handles[h], ", ")+")")
			}
		}
		return strings.Join(parts, ", ")
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// ---------------------------------------------------------------- help

func (s *shell) helpText() string {
	p := s.p
	key := func(k, d string) string { return p.Fg(ui.Green, ui.PadRight(k, 12)) + ui.PadRight(d, 26) }
	lines := []string{
		"",
		"  " + p.Bold("Without typing commands"),
		"    " + p.Fg(ui.Green, ui.PadRight("↓ on an empty line", 22)) + p.Muted("quick actions: your domains, register, DNS, balance, environment"),
		"    " + p.Fg(ui.Green, ui.PadRight("type a name", 22)) + p.Muted("acme → which extensions are free and their prices; ↓ picks one"),
		"    " + p.Fg(ui.Green, ui.PadRight("enter on a domain", 22)) + p.Muted("its menu: details, DNS, renew, lock, privacy, nameservers, auth code"),
		"    " + p.Fg(ui.Green, ui.PadRight("/", 22)) + p.Muted("every shell command: /domains /register /dns /renew /balance /env /mcp"),
		"",
		"  " + p.Bold("Keys"),
		"    " + key("↑ ↓", "choose, or history") + key("enter", "open, run, next field"),
		"    " + key("tab", "complete") + key("← →", "change a choice"),
		"    " + key("esc", "back one step, cancel") + key("ctrl+l", "clear the screen"),
		"    " + key("ctrl+c ×2", "exit") + key("?", "this help"),
		"",
		"  " + p.Bold("Also works"),
		"    " + p.Muted("any dna command: ") + p.Command("domain info acme.com") + p.Muted(", ") + p.Command("dns list acme.com --jq '.[].name'"),
		"",
		"  " + p.Bold("Safety"),
		"    " + p.Muted("Billable and destructive steps end in a confirmation box; Cancel is selected until you choose otherwise."),
		"    " + p.Command("/env ote") + p.Muted(" switches to the test environment, where nothing costs real money."),
		"",
	}
	return strings.Join(lines, "\n")
}

// splitWords splits a command line like a shell: quotes group words.
func splitWords(line string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("missing closing %c quote", quote)
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}
