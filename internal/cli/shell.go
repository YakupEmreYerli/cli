package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/domainreseller/cli/internal/output"
	"github.com/domainreseller/cli/internal/ui"
)

// The interactive shell is what a person gets when they run `dna` on its own
// in a terminal. Nothing has to be memorised:
//
//   - an empty prompt shows quick actions (↓ to choose),
//   - typing a name checks it live; ↓ picks a result and offers what to do,
//   - anything that needs a domain opens a searchable list of your domains,
//   - a domain opens an action menu (details, DNS, renew, lock, ...),
//   - registrations, renewals, DNS records and nameservers are short forms,
//   - billable and destructive steps end in a confirmation box.
//
// Typing dna commands still works for people who know them. Scripts and AI
// agents never see the shell (they get the help text, see App.human).

type focusArea int

const (
	focusInput focusArea = iota
	focusHome            // quick actions under an empty prompt
	focusLive            // live search results
)

type shell struct {
	app  *App
	p    ui.Painter
	in   textinput.Model
	root *cobra.Command // command tree, for completion

	width  int
	env    string   // environment chosen with /env ("" = configured)
	base   []string // global flags given to `dna` itself, passed to every command
	status statusInfo
	prefs  shellPrefs

	stack []overlay // menus, forms and boxes shown instead of the prompt

	focus   focusArea
	homeSel int
	liveSel int

	sugg       []suggestion
	suggSel    int
	suggHidden bool
	suggNav    bool // the user moved through the suggestions with ↑↓

	live    liveState
	run     runState
	domains domainCache

	pendingNote string // extra line for the next confirmation box (a price)

	history []string
	histPos int
	draft   string

	quitArmed time.Time
	notice    string
	frame     int
	ticking   bool
}

type statusInfo struct {
	loaded   bool
	env      string
	profile  string
	readOnly bool
	acct     *account
	err      error
}

type suggestion struct {
	value string // the input after accepting it
	label string // what the menu shows
	desc  string
	run   bool // Enter runs the value right away
}

type runState struct {
	active bool
	label  string
	start  time.Time
	cancel context.CancelFunc
}

// shellPrefs are remembered between sessions (next to the config file).
type shellPrefs struct {
	Handle      string `json:"handle,omitempty"`
	Nameservers string `json:"nameservers,omitempty"`
}

type acctMsg statusInfo
type frameMsg struct{}

// canRunShell reports whether `dna` on its own opens the interactive shell:
// a person at a real terminal (stdin and stdout), not an AI agent, and not
// the shell itself.
func (a *App) canRunShell() bool {
	if a.inShell || !a.human() || !a.StdinTTY {
		return false
	}
	if f, err := a.format(); err != nil || f != output.Table || a.flags.jq != "" {
		return false
	}
	_, inFile := a.In.(*os.File)
	_, outFile := a.Out.(*os.File)
	return inFile && outFile
}

// runShell starts the interactive shell.
func (a *App) runShell(ctx context.Context) error {
	s := newShell(a)
	prog := tea.NewProgram(s, tea.WithContext(ctx))
	_, err := prog.Run()
	s.saveHistory()
	if err == tea.ErrProgramKilled || err == context.Canceled {
		return nil
	}
	return err
}

// cursorBlink makes text inputs blink their cursor (off in tests).
var cursorBlink = true

func newTextInput(value, placeholder string) textinput.Model {
	in := textinput.New()
	if !cursorBlink {
		in.Cursor.SetMode(cursor.CursorStatic)
	}
	in.Prompt = ""
	in.Placeholder = placeholder
	in.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A93B0"))
	in.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#1CCC99"))
	in.CharLimit = 2000
	in.SetValue(value)
	return in
}

func newShell(a *App) *shell {
	in := newTextInput("", "Type a domain name to search it, or press ↓ for quick actions")
	in.Focus()
	s := &shell{app: a, p: a.outP(), in: in, width: a.width()}
	if s.width == 0 {
		s.width = 80
	}
	s.base = a.baseArgs()
	s.env = a.flags.env
	if a.flags.ote {
		s.env = "ote"
	}
	s.root = a.child(nil, nil, 0).newRootCmd()
	s.loadHistory()
	s.loadPrefs()
	return s
}

// baseArgs turns the global flags given to `dna` into arguments for every
// command the shell runs, so `dna --profile x` keeps using profile x.
func (a *App) baseArgs() []string {
	var args []string
	f := a.flags
	add := func(name, v string) {
		if v != "" {
			args = append(args, "--"+name, v)
		}
	}
	add("profile", f.profile)
	add("config", f.configPath)
	add("base-url", f.baseURL)
	add("reseller-id", f.resellerID)
	add("api-key", f.apiKey)
	if f.readOnly {
		args = append(args, "--read-only")
	}
	if f.noColor {
		args = append(args, "--no-color")
	}
	return args
}

func (s *shell) Init() tea.Cmd {
	return tea.Batch(tea.Println(s.welcome()), s.fetchStatus(), textinput.Blink)
}

func (s *shell) welcome() string {
	p := s.p
	w := s.width - 1
	if w < 30 {
		w = 30
	}
	inner := w - 4
	// center a block of lines as a whole, keeping it left-aligned inside
	center := func(block []string) []string {
		bw := 0
		for _, l := range block {
			if x := ui.Width(l); x > bw {
				bw = x
			}
		}
		pad := ""
		if inner > bw {
			pad = strings.Repeat(" ", (inner-bw)/2)
		}
		out := make([]string, len(block))
		for i, l := range block {
			out[i] = pad + l
		}
		return out
	}
	lines := []string{""}
	lines = append(lines, center(p.Logo(inner))...)
	lines = append(lines, "")
	ver := p.Fg(ui.Green, displayVersion(s.app.Build.Version))
	if inner >= 80 {
		lines = append(lines, center([]string{p.Muted("Domains, DNS, SSL and hosting from your terminal  ·  ") + ver})...)
		lines = append(lines, "")
		lines = append(lines, center([]string{
			p.Fg(ui.Green, "▸ ") + "Type a name" + p.Muted(" to see which extensions are free, then ") + p.Fg(ui.Green, "↓") + p.Muted(" to pick one"),
			p.Fg(ui.Green, "▸ ") + "Press " + p.Fg(ui.Green, "↓") + p.Muted(" on an empty line for quick actions: your domains, DNS, balance"),
			p.Fg(ui.Green, "▸ ") + p.Command("/") + p.Muted(" lists every command · ") + p.Command("?") + p.Muted(" shows the keys · ") + p.Command("Esc") + p.Muted(" always goes back"),
		})...)
	} else {
		// narrow terminals: the same, shorter
		lines = append(lines, center([]string{p.Muted("Domains · DNS · SSL · hosting  ·  ") + ver})...)
		lines = append(lines, "")
		lines = append(lines, center([]string{
			p.Fg(ui.Green, "▸ ") + "Type a name" + p.Muted(": free extensions, prices"),
			p.Fg(ui.Green, "▸ ") + p.Fg(ui.Green, "↓") + p.Muted(" on an empty line: quick actions"),
			p.Fg(ui.Green, "▸ ") + p.Command("/") + p.Muted(" commands · ") + p.Command("?") + p.Muted(" keys · ") + p.Command("Esc") + p.Muted(" back"),
		})...)
	}
	lines = append(lines, "")
	box := p.Box(lines, w, ui.GreenDeep, p.BoldFg(ui.Green, "Domain Name API")+p.Muted(" · dna"))
	return "\n" + strings.Join(box, "\n") + "\n"
}

// ---------------------------------------------------------------- update

func (s *shell) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = m.Width
		return s, nil
	case acctMsg:
		s.status = statusInfo(m)
		s.status.loaded = true
		return s, nil
	case frameMsg:
		s.ticking = false
		if s.busy() {
			s.frame++
			return s, s.tick()
		}
		return s, nil
	case liveDebounceMsg:
		return s, s.onLiveDebounce(m)
	case liveResultMsg:
		s.onLiveResult(m)
		return s, nil
	case runDoneMsg:
		return s, s.onRunDone(m)
	case dryRunMsg:
		return s, s.onDryRun(m)
	case contactStepMsg:
		return s, s.onContactStep(m)
	case contactsFoundMsg:
		s.onContactsFound(m)
		return s, nil
	case domainsMsg:
		return s, s.onDomains(m)
	case availMsg:
		return s, s.onAvail(m)
	case execDoneMsg:
		note := m.note
		if note == "" {
			note = s.p.Muted("  back in the dna shell")
		}
		if st, ok := s.top().(*agentSetup); ok {
			st.creds = nil // a login may have added credentials
		}
		return s, tea.Batch(s.fetchStatus(), tea.Println(note))
	case tea.KeyMsg:
		if m.Type == tea.KeyCtrlC && s.run.active {
			s.run.cancel()
			return s, nil
		}
		if o := s.top(); o != nil {
			s.notice = ""
			return s, o.key(s, m)
		}
		return s, s.promptKey(m)
	}
	// cursor blinking and other messages go to the active text input
	var cmds []tea.Cmd
	var cmd tea.Cmd
	s.in, cmd = s.in.Update(msg)
	cmds = append(cmds, cmd)
	if f, ok := s.top().(*form); ok {
		cmds = append(cmds, f.update(msg))
	}
	return s, tea.Batch(cmds...)
}

func (s *shell) busy() bool {
	return s.run.active || s.live.busy || s.domains.state == domainsLoading
}

// tick animates spinners while something is in progress.
func (s *shell) tick() tea.Cmd {
	if s.ticking {
		return nil
	}
	s.ticking = true
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg { return frameMsg{} })
}

// ---------------------------------------------------------------- view

func (s *shell) View() string {
	w := s.width - 1
	if w < 30 {
		w = 30
	}
	var out []string
	if s.run.active {
		out = append(out, s.runningLine())
	}
	if o := s.top(); o != nil {
		out = append(out, o.view(s, w)...)
	} else {
		s.in.Width = w - 7
		if w < 80 {
			s.in.Placeholder = "Type a domain, or ↓ for actions"
		} else {
			s.in.Placeholder = "Type a domain name to search it, or press ↓ for quick actions"
		}
		border := ui.GreenDeep
		if s.run.active || s.focus != focusInput {
			border = ui.Muted
		}
		mark := s.p.BoldFg(ui.Green, "›")
		if s.focus != focusInput {
			mark = s.p.Muted("›")
		}
		out = append(out, s.p.Box([]string{mark + " " + s.in.View()}, w, border, "")...)
		out = append(out, s.belowPrompt(w)...)
	}
	if s.notice != "" {
		out = append(out, "  "+s.p.Fg(ui.Amber, s.notice))
	}
	out = append(out, s.statusLine(w))
	// A line wider than the terminal would wrap and break the layout.
	for i, l := range out {
		if ui.Width(l) > w {
			out[i] = ui.TruncateStyled(l, w)
		}
	}
	return strings.Join(out, "\n")
}

func (s *shell) runningLine() string {
	p := s.p
	frame := ui.SpinnerFrames[s.frame%len(ui.SpinnerFrames)]
	el := time.Since(s.run.start).Round(100 * time.Millisecond)
	return " " + p.Fg(ui.Green, frame) + " " + p.Shimmer(s.run.label+"…", s.frame) + p.Muted(fmt.Sprintf("  %s · esc to cancel", el))
}

func (s *shell) currentEnv() string {
	if s.env != "" {
		return s.env
	}
	return s.status.env
}

func (s *shell) statusLine(w int) string {
	p := s.p
	st := s.status
	var left string
	switch {
	case !st.loaded:
		left = p.Muted("connecting…")
	default:
		if strings.EqualFold(s.currentEnv(), "ote") {
			left = p.Badge(ui.Green, "OTE") + " " + p.Muted("test")
		} else {
			left = p.Badge(ui.Red, "PRODUCTION") + " " + p.Muted("real money")
		}
		if st.readOnly {
			left += " " + p.Badge(ui.Amber, "READ-ONLY")
		}
		switch {
		case st.acct != nil:
			usd := p.Fg(ui.Green, fmtMoney(st.acct.UsdBalance)+" USD")
			try := p.Muted(" · ") + p.Fg(ui.Green, fmtMoney(st.acct.TryBalance)+" TRY")
			name := p.Muted("  ·  ") + p.Bold(st.acct.ResellerName)
			// drop the least important parts on a narrow terminal
			switch {
			case ui.Width(left+name+p.Muted("  ·  ")+usd+try) <= w-2:
				left += name + p.Muted("  ·  ") + usd + try
			case ui.Width(left+name+p.Muted("  ·  ")+usd) <= w-2:
				left += name + p.Muted("  ·  ") + usd
			default:
				left += p.Muted("  ·  ") + usd
			}
		case st.err != nil:
			left += p.Muted("  ·  ") + p.Fg(ui.Amber, "▲ "+shortErr(st.err)) + p.Muted(" · log in with ") + p.Command("/login")
		}
	}
	right := p.Muted("? keys · ctrl+c ×2 exit")
	gap := w - ui.Width(left) - ui.Width(right) - 2
	if gap < 2 {
		return " " + left
	}
	return " " + left + strings.Repeat(" ", gap) + right
}

func shortErr(err error) string {
	p := classify(err)
	switch p.Kind {
	case "auth":
		if p.Code == "NO_CREDENTIALS" {
			return "not logged in"
		}
		return "credentials rejected"
	case "network", "timeout":
		return "API unreachable"
	}
	return ui.Truncate(p.Message, 40)
}

// ---------------------------------------------------------------- history and prefs

func (s *shell) stateFile(name string) string {
	return filepath.Join(filepath.Dir(s.app.configPath()), name)
}

func (s *shell) loadHistory() {
	b, err := os.ReadFile(s.stateFile("history"))
	if err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				s.history = append(s.history, l)
			}
		}
	}
	s.histPos = len(s.history)
}

func (s *shell) saveHistory() {
	h := s.history
	if len(h) > 500 {
		h = h[len(h)-500:]
	}
	_ = os.MkdirAll(filepath.Dir(s.stateFile("history")), 0o700)
	_ = os.WriteFile(s.stateFile("history"), []byte(strings.Join(h, "\n")+"\n"), 0o600)
}

func (s *shell) loadPrefs() {
	if b, err := os.ReadFile(s.stateFile("shell.json")); err == nil {
		_ = json.Unmarshal(b, &s.prefs)
	}
}

func (s *shell) savePrefs() {
	b, _ := json.MarshalIndent(s.prefs, "", "  ")
	_ = os.MkdirAll(filepath.Dir(s.stateFile("shell.json")), 0o700)
	_ = os.WriteFile(s.stateFile("shell.json"), b, 0o600)
}

func (s *shell) remember(line string) {
	if strings.Contains(line, "--api-key") || line == "" {
		return // never store secrets
	}
	if n := len(s.history); n > 0 && s.history[n-1] == line {
		s.histPos = len(s.history)
		return
	}
	s.history = append(s.history, line)
	s.histPos = len(s.history)
}

func (s *shell) historyMove(up bool) {
	if len(s.history) == 0 {
		return
	}
	if s.histPos == len(s.history) {
		s.draft = s.in.Value()
	}
	if up && s.histPos > 0 {
		s.histPos--
	} else if !up && s.histPos < len(s.history) {
		s.histPos++
	}
	v := s.draft
	if s.histPos < len(s.history) {
		v = s.history[s.histPos]
	}
	s.in.SetValue(v)
	s.in.CursorEnd()
	s.suggHidden = true
	s.live = liveState{}
}
