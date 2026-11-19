package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/domainreseller/cli/internal/ui"
)

// copyText puts text on the clipboard (replaced in tests).
var copyText = clipboard.WriteAll

// "Connect an AI agent": pick the agent, choose the environment and how much
// it may do, then let dna set it up (or copy the exact snippet), check the
// credentials it will use and hand over ready-made prompts to try.

func (s *shell) agentPicker() tea.Cmd {
	var items []menuItem
	for _, a := range agents {
		ag := a
		desc := ag.Desc
		if ag.found() {
			desc = "✓ installed · " + desc
		}
		items = append(items, menuItem{label: ag.Name, desc: desc, opens: true, act: func(s *shell) tea.Cmd {
			s.push(s.newAgentSetup(ag))
			return nil
		}})
	}
	s.push(&menu{
		title:      s.p.BoldFg(ui.Green, "Connect an AI agent") + s.p.Muted(" · which one do you use?"),
		items:      items,
		filterable: true,
		footer:     "↑↓ choose · enter open · type to search · esc back",
	})
	return nil
}

var envChoices = []struct{ id, label string }{
	{"ote", "OTE, the test environment (recommended to start)"},
	{"production", "Production (real domains, real money)"},
}

var accessChoices = []struct {
	label              string
	readOnly, noBilled bool
}{
	{"Read-only: it can look, never change anything", true, false},
	{"No purchases: it can change DNS and settings, never buy", false, true},
	{"Everything: purchases still need a confirmation", false, false},
}

type agentSetup struct {
	ag     agentInfo
	env    int
	access int
	row    int
	found  bool
	bin    string
	msg    string
	tone   ui.Tone
	creds  map[string]credCheck // per environment, cleared after a login
}

type credCheck struct {
	ok   bool
	note string
	tone ui.Tone
}

// cred checks the credentials of the selected environment once.
func (st *agentSetup) cred(s *shell) credCheck {
	env := envChoices[st.env].id
	if st.creds == nil {
		st.creds = map[string]credCheck{}
	}
	c, ok := st.creds[env]
	if !ok {
		c.ok, c.note, c.tone = s.credentials(env)
		st.creds[env] = c
	}
	return c
}

func (s *shell) newAgentSetup(a agentInfo) *agentSetup {
	st := &agentSetup{ag: a, found: a.found(), bin: dnaPath(), access: 2}
	if !strings.EqualFold(s.currentEnv(), "ote") {
		st.env, st.access = 1, 0 // production: start read-only
	}
	return st
}

func (st *agentSetup) chat() bool { return st.ag.Kind == kindChat }

func (st *agentSetup) launch(s *shell) mcpLaunch {
	acc := accessChoices[st.access]
	l := mcpLaunchFor(envChoices[st.env].id, s.app.flags.profile, acc.readOnly, acc.noBilled)
	l.Bin = st.bin
	return l
}

type setupAction struct {
	label string
	run   func(s *shell, st *agentSetup) tea.Cmd
}

func (st *agentSetup) actions(s *shell) []setupAction {
	a := st.ag
	var out []setupAction
	if st.chat() {
		if a.ID == "chatgpt" {
			out = append(out, setupAction{"Set up OpenAI Codex instead (ChatGPT's agent that runs dna for you)", func(s *shell, _ *agentSetup) tea.Cmd {
				codex, _ := lookupAgent("codex")
				s.push(s.newAgentSetup(codex))
				return nil
			}})
		}
		return append(out, setupAction{"Ready-made prompts to paste into " + a.Name, func(s *shell, st *agentSetup) tea.Cmd { return s.promptMenu(st) }})
	}
	env := envChoices[st.env].id
	if !st.cred(s).ok {
		out = append(out, setupAction{"Log in to " + strings.ToUpper(env) + " first (the agent needs these credentials)", func(s *shell, _ *agentSetup) tea.Cmd {
			return s.execInteractive("auth", "login", "--env", env)
		}})
	}
	l := st.launch(s)
	switch {
	case a.Kind == kindCommand && st.found:
		out = append(out, setupAction{"Run the setup command for me", func(s *shell, st *agentSetup) tea.Cmd {
			cmd := l.addCommand(a)
			return s.execProgram(exec.Command(cmd[0], cmd[1:]...), "dna was added to "+a.Name+". "+a.Then)
		}})
	case a.ID == "vscode" && st.found && runtime.GOOS != "windows":
		out = append(out, setupAction{"Add it to VS Code for me", func(s *shell, st *agentSetup) tea.Cmd {
			cmd := l.addCommand(a)
			return s.execProgram(exec.Command(cmd[0], cmd[1:]...), "dna was added to VS Code. "+a.Then)
		}})
	}
	if a.ID == "cursor" {
		out = append(out, setupAction{"Open the one-click install link in Cursor", func(s *shell, st *agentSetup) tea.Cmd {
			if err := openURL(l.cursorLink()); err != nil {
				st.msg, st.tone = "Could not open the link: "+err.Error(), ui.Bad
				return nil
			}
			st.msg, st.tone = "Cursor should ask you to install dna now. "+a.Then, ui.Good
			return nil
		}})
	}
	if a.writable() {
		out = append(out, setupAction{"Add it to " + a.configFile() + " for me (keeps your other servers)", func(s *shell, st *agentSetup) tea.Cmd {
			path, backup, err := l.installConfig(a)
			if err != nil {
				st.msg, st.tone = err.Error(), ui.Bad
				return nil
			}
			st.msg, st.tone = "Added to "+path, ui.Good
			if backup != "" {
				st.msg += " (the previous version is in " + backup + ")"
			}
			if a.Then != "" {
				st.msg += ". " + a.Then
			}
			return nil
		}})
	}
	out = append(out,
		setupAction{"Copy the setup to the clipboard", func(s *shell, st *agentSetup) tea.Cmd {
			text, _ := l.snippet(a)
			if err := copyText(text); err != nil {
				st.msg, st.tone = "The clipboard is not available here; select the text above instead.", ui.Warn
				return nil
			}
			st.msg, st.tone = "Copied. Paste it as shown above.", ui.Good
			return nil
		}},
		setupAction{"Ready-made prompts to try once it is connected", func(s *shell, st *agentSetup) tea.Cmd { return s.promptMenu(st) }},
	)
	return out
}

// credentials reports whether dna finds credentials for env on its own (in a
// profile), as an agent starting dna will need.
func (s *shell) credentials(env string) (ok bool, note string, tone ui.Tone) {
	c := s.app.child(nil, nil, 0)
	c.flags.env, c.flags.ote = env, false
	r, err := c.resolved()
	if err != nil {
		return false, err.Error(), ui.Bad
	}
	if !r.HasCredentials() {
		return false, "No credentials for " + strings.ToUpper(env) + " yet: log in first, the agent uses the same ones.", ui.Warn
	}
	if strings.HasPrefix(r.APIKeySource, "env ") || strings.HasPrefix(r.APIKeySource, "flag") {
		return true, "Your API key comes from this terminal (" + r.APIKeySource + "); apps started elsewhere will not see it. Log in once with dna auth login --env " + env + ".", ui.Warn
	}
	return true, "Credentials for " + strings.ToUpper(env) + " are stored (" + r.APIKeySource + "); the agent uses them, the key never goes into its config.", ui.Good
}

func (st *agentSetup) rows(s *shell) int {
	n := len(st.actions(s))
	if !st.chat() {
		n += 2
	}
	return n
}

func (st *agentSetup) key(s *shell, k tea.KeyMsg) tea.Cmd {
	n := st.rows(s)
	choice := !st.chat() && st.row < 2
	switch k.String() {
	case "esc", "q":
		s.pop()
		return nil
	case "up", "shift+tab", "k":
		if st.row > 0 {
			st.row--
		}
	case "down", "tab", "j":
		if st.row < n-1 {
			st.row++
		}
	case "left", "right", "h", "l":
		if !choice {
			return nil
		}
		d := 1
		if k.String() == "left" || k.String() == "h" {
			d = -1
		}
		if st.row == 0 {
			st.env = (st.env + d + len(envChoices)) % len(envChoices)
		} else {
			st.access = (st.access + d + len(accessChoices)) % len(accessChoices)
		}
		st.msg = ""
	case "enter":
		if choice {
			st.row++
			return nil
		}
		i := st.row
		if !st.chat() {
			i -= 2
		}
		acts := st.actions(s)
		if i >= 0 && i < len(acts) {
			st.msg = ""
			return acts[i].run(s, st)
		}
	}
	return nil
}

func (st *agentSetup) view(s *shell, w int) []string {
	p := s.p
	a := st.ag
	bw := min(w, 110)
	inner := bw - 4
	var lines []string
	choiceRow := func(i int, label, value string) string {
		if st.row == i {
			return p.BoldFg(ui.Green, "›") + " " + p.Bold(ui.PadRight(label, 12)) + " " + p.Fg(ui.Green, "‹ ") + p.Bold(value) + p.Fg(ui.Green, " ›")
		}
		return "  " + p.Muted(ui.PadRight(label, 12)) + " " + value
	}
	if st.chat() {
		lines = append(lines, chatIntro(p, a)...)
	} else {
		lines = append(lines,
			choiceRow(0, "Environment", envChoices[st.env].label),
			choiceRow(1, "Access", accessChoices[st.access].label),
			"",
		)
		l := st.launch(s)
		text, _ := l.snippet(a)
		step := 1
		say := func(s string) {
			for i, l := range wrapCode(s, inner-3) {
				if i == 0 {
					lines = append(lines, p.Fg(ui.Green, fmt.Sprintf("%d. ", step))+l)
				} else {
					lines = append(lines, "   "+l)
				}
			}
			step++
		}
		switch a.Kind {
		case kindCommand:
			say("Add dna to " + a.Name + ", for all your projects:")
		case kindJSON, kindTOML, kindYAML:
			say("Add this to " + p.Bold(a.configFile()) + ":")
		case kindSteps:
			for _, s := range a.Steps {
				say(s)
			}
		}
		for _, line := range wrapCode(text, inner-4) {
			lines = append(lines, "   "+p.Fg(ui.NavyTint, line))
		}
		if a.Then != "" {
			say(a.Then)
		}
		lines = append(lines, "")
		if a.Exe != "" {
			if st.found {
				lines = append(lines, p.Fg(ui.Green, "✓ ")+p.Muted(a.Exe+" is installed on this computer"))
			} else {
				lines = append(lines, p.Fg(ui.Amber, "▲ ")+p.Muted(a.Exe+" was not found on this computer; install "+a.Name+" first, or copy the setup"))
			}
		}
		cc := st.cred(s)
		note, tone := cc.note, cc.tone
		mark := "✓ "
		if tone != ui.Good {
			mark = "▲ "
		}
		for i, l := range wrapCode(note, inner-2) {
			if i == 0 {
				lines = append(lines, p.Fg(tone.Color(), mark)+p.Muted(l))
			} else {
				lines = append(lines, "  "+p.Muted(l))
			}
		}
		lines = append(lines, "")
	}
	acts := st.actions(s)
	for i, act := range acts {
		row := i
		if !st.chat() {
			row += 2
		}
		if st.row == row {
			lines = append(lines, p.BoldFg(ui.Green, "› "+act.label))
		} else {
			lines = append(lines, "  "+act.label)
		}
	}
	if st.msg != "" {
		lines = append(lines, "")
		for _, l := range wrapCode(st.msg, inner-2) {
			lines = append(lines, p.Fg(st.tone.Color(), l))
		}
	}
	title := p.Bold("Connect ") + p.BoldFg(ui.Green, a.Name)
	out := p.Box(lines, bw, ui.GreenDeep, title)
	hint := "↑↓ choose · enter run · esc back"
	if !st.chat() {
		hint = "↑↓ choose · ←→ change environment / access · enter run · esc back"
	}
	return append(out, s.hint(hint))
}

func chatIntro(p ui.Painter, a agentInfo) []string {
	if a.ID == "chatgpt" {
		return []string{
			"ChatGPT in the browser and in the ChatGPT app cannot start programs on your",
			"computer, so it cannot run dna by itself. Two ways that work:",
			"",
			p.Fg(ui.Green, "1. ") + p.Bold("OpenAI Codex") + p.Muted(" (comes with ChatGPT plans) runs on your computer and"),
			"   " + p.Muted("uses dna as a tool: it searches, checks and prepares changes for you."),
			p.Fg(ui.Green, "2. ") + p.Bold("A ready-made prompt") + p.Muted(": ChatGPT reads the dna docs and gives you the"),
			"   " + p.Muted("exact commands; you run them here."),
			"",
		}
	}
	return []string{
		"Chat assistants in the browser cannot run programs on your computer. Paste a",
		"ready-made prompt: the assistant reads the dna docs and gives you the exact",
		"commands, which you run here.",
		"",
	}
}

// wrapCode breaks long lines (paths, JSON) at the width without losing text.
func wrapCode(text string, width int) []string {
	if width < 20 {
		width = 20
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		for ui.Width(line) > width {
			r := []rune(line)
			cut := width
			if cut > len(r) {
				cut = len(r)
			}
			// prefer breaking at a space, if there is one in the second half
			if sp := strings.LastIndex(string(r[:cut]), " "); sp > width*2/5 {
				cut = len([]rune(string(r[:cut])[:sp])) + 1
			}
			out = append(out, strings.TrimRight(string(r[:cut]), " "))
			line = string(r[cut:])
		}
		out = append(out, line)
	}
	return out
}

// execProgram hands the terminal to another program (such as claude mcp
// add) and reports the outcome afterwards.
func (s *shell) execProgram(c *exec.Cmd, success string) tea.Cmd {
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return execDoneMsg{err: err, note: "  " + s.p.Fg(ui.Red, "✗ ") + strings.Join(c.Args[:min(len(c.Args), 3)], " ") + " failed: " + err.Error()}
		}
		return execDoneMsg{note: "  " + s.p.BoldFg(ui.Green, "✓ ") + success}
	})
}

func openURL(u string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		c = exec.Command("open", u)
	default:
		c = exec.Command("xdg-open", u)
	}
	return c.Start()
}

// ---------------------------------------------------------------- prompts

type readyPrompt struct {
	title string
	text  string
}

func readyPrompts(a agentInfo, env string) []readyPrompt {
	envName := "the OTE test environment"
	if env == "production" {
		envName = "production"
	}
	var out []readyPrompt
	if a.Kind == kindChat {
		out = append(out, readyPrompt{"Give me the commands, step by step",
			"Read https://raw.githubusercontent.com/domainreseller/cli/main/llms.txt and the dna\ndocs it links to. Then give me the exact dna commands, step by step, to register a\ndomain in the OTE test environment: install, log in, search, preview with `--dry-run`\nand register. Explain what each command does."})
		out = append(out, readyPrompt{"Explain what dna can do for my reseller business",
			"Read https://raw.githubusercontent.com/domainreseller/cli/main/llms.txt. Explain in\nplain words what I can do with the dna CLI as a domain reseller, with one example\ncommand for each task. Mention which commands cost money and how --dry-run helps."})
		return out
	}
	out = append(out,
		readyPrompt{"Check that dna works",
			"With dna, show my reseller account and balance in " + envName + " and list my domains\nwith their expiry dates. Don't change anything; just tell me if something looks wrong."},
		readyPrompt{"Find and register a domain",
			"With dna, check whether brandname.com, brandname.net and brandname.io are available.\nShow the results with prices in a table, then preview the registration of the cheapest\navailable one with `--dry-run`. Don't register anything until I say yes."},
		readyPrompt{"Upcoming renewals",
			"With dna, list my domains that expire in the next 60 days with their renewal prices,\nsorted by expiry date. Don't renew anything."},
		readyPrompt{"Point a domain to a server",
			"With dna, point example.com to 203.0.113.10: add A records for `@` and `www`, then\nshow me the final DNS zone. Ask me before changing or deleting any existing record."},
		readyPrompt{"Transfer a domain in",
			"With dna, check whether example.org can be transferred to my account, tell me what I\nstill need (auth code, unlocking) and preview the transfer with `--dry-run`."},
		readyPrompt{"Order an SSL certificate",
			"With dna, find the cheapest DV SSL certificate for example.com, create a CSR and\npreview the order. Wait for my approval before buying."},
		readyPrompt{"Enable DNSSEC",
			"With dna, show me the DNSSEC status of example.com, then add the DS record I give you\nfrom my DNS provider. Preview the change with `--dry-run` first and wait for my\napproval before applying it."},
		readyPrompt{"Weekly health check",
			"With dna, check my account: domains expiring within 30 days, domains without transfer\nlock, domains waiting for documents, and my balance. Summarise what needs my attention\nas a short to-do list. Don't change anything."},
	)
	return out
}

func (s *shell) promptMenu(st *agentSetup) tea.Cmd {
	env := envChoices[st.env].id
	var items []menuItem
	for _, pr := range readyPrompts(st.ag, env) {
		pr := pr
		first := strings.SplitN(pr.text, "\n", 2)[0]
		items = append(items, menuItem{label: pr.title, desc: first, opens: true, act: func(s *shell) tea.Cmd {
			copied := copyText(pr.text) == nil
			head := "  " + s.p.BoldFg(ui.Green, "✓ ") + "Copied. Paste it into " + st.ag.Name + ":"
			if !copied {
				head = "  " + s.p.Fg(ui.Amber, "▲ ") + "Select and copy this prompt into " + st.ag.Name + ":"
			}
			box := s.p.Box(strings.Split(pr.text, "\n"), 0, ui.GreenDeep, s.p.Bold(pr.title))
			return tea.Println("\n" + head + "\n" + strings.Join(box, "\n"))
		}})
	}
	s.push(&menu{
		title:  s.p.Bold("Ready-made prompts") + s.p.Muted(" · enter copies one"),
		items:  items,
		footer: "↑↓ choose · enter copy and show it · esc back",
	})
	return nil
}
