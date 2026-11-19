package cli

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/domainreseller/cli/internal/ui"
)

// ---------------------------------------------------------------- keys

func (s *shell) promptKey(k tea.KeyMsg) tea.Cmd {
	s.notice = ""
	switch s.focus {
	case focusHome:
		if cmd, handled := s.homeKey(k); handled {
			return cmd
		}
	case focusLive:
		if cmd, handled := s.liveKey(k); handled {
			return cmd
		}
	}
	switch k.Type {
	case tea.KeyCtrlC:
		if s.in.Value() != "" {
			s.setInput("")
			return nil
		}
		if time.Since(s.quitArmed) < 2*time.Second {
			return tea.Quit
		}
		s.quitArmed = time.Now()
		s.notice = "Press Ctrl+C again to exit"
		return nil
	case tea.KeyCtrlD:
		if s.in.Value() == "" {
			return tea.Quit
		}
	case tea.KeyCtrlL:
		return tea.Sequence(tea.ClearScreen, tea.Println(s.welcome()))
	case tea.KeyEsc:
		if s.run.active {
			s.run.cancel()
			return nil
		}
		if s.menuOpen() {
			s.suggHidden = true
			return nil
		}
		s.setInput("")
		return nil
	case tea.KeyEnter:
		return s.enter()
	case tea.KeyTab:
		if s.menuOpen() {
			s.setInput(s.sugg[s.suggSel].value)
			return s.onInputChange()
		}
		if d := s.live.firstAvailable(); d != "" {
			s.setInput(d)
			return s.onInputChange()
		}
		return nil
	case tea.KeyUp:
		if s.menuOpen() {
			s.suggNav = true
			s.suggSel = (s.suggSel + len(s.sugg) - 1) % len(s.sugg)
			return nil
		}
		s.historyMove(true)
		return nil
	case tea.KeyDown:
		if s.menuOpen() {
			s.suggNav = true
			s.suggSel = (s.suggSel + 1) % len(s.sugg)
			return nil
		}
		if s.liveReady() {
			s.focus, s.liveSel = focusLive, 0
			return nil
		}
		if strings.TrimSpace(s.in.Value()) == "" {
			s.focus, s.homeSel = focusHome, 0
			return nil
		}
		s.historyMove(false)
		return nil
	case tea.KeyRunes:
		if string(k.Runes) == "?" && s.in.Value() == "" {
			return tea.Println(s.helpText())
		}
	}
	before := s.in.Value()
	var cmd tea.Cmd
	s.in, cmd = s.in.Update(k)
	if s.in.Value() != before {
		return tea.Batch(cmd, s.onInputChange())
	}
	return cmd
}

// enter runs the input, or the highlighted suggestion where that is what the
// user clearly meant.
func (s *shell) enter() tea.Cmd {
	if s.run.active {
		s.notice = "Still working on the previous command. Esc cancels it."
		return nil
	}
	line := strings.TrimSpace(s.in.Value())
	if line == "" {
		s.focus, s.homeSel = focusHome, 0
		return nil
	}
	if s.menuOpen() {
		sg := s.sugg[s.suggSel]
		switch {
		case sg.run && (strings.HasPrefix(line, "/") || s.suggNav):
			line = strings.TrimSpace(sg.value)
		case s.suggNav:
			s.setInput(sg.value)
			return s.onInputChange()
		}
	}
	s.setInput("")
	return s.submit(line)
}

func (s *shell) homeKey(k tea.KeyMsg) (tea.Cmd, bool) {
	items := s.homeItems()
	switch k.Type {
	case tea.KeyUp:
		if s.homeSel > 0 {
			s.homeSel--
		} else {
			s.focus = focusInput
		}
		return nil, true
	case tea.KeyDown, tea.KeyTab:
		if s.homeSel < len(items)-1 {
			s.homeSel++
		}
		return nil, true
	case tea.KeyEsc:
		s.focus = focusInput
		return nil, true
	case tea.KeyEnter:
		s.focus = focusInput
		return s.choose(items[s.homeSel]), true
	case tea.KeyRunes:
		if n, err := strconv.Atoi(string(k.Runes)); err == nil && n >= 1 && n <= len(items) {
			s.focus = focusInput
			return s.choose(items[n-1]), true
		}
	}
	s.focus = focusInput // typing goes back to the prompt
	return nil, false
}

func (s *shell) liveKey(k tea.KeyMsg) (tea.Cmd, bool) {
	res := s.live.results
	switch k.Type {
	case tea.KeyUp:
		if s.liveSel > 0 {
			s.liveSel--
		} else {
			s.focus = focusInput
		}
		return nil, true
	case tea.KeyDown:
		if s.liveSel < len(res)-1 {
			s.liveSel++
		}
		return nil, true
	case tea.KeyEsc:
		s.focus = focusInput
		return nil, true
	case tea.KeyTab:
		if s.liveSel < len(res) {
			name, _ := res[s.liveSel]["domainName"].(string)
			s.focus = focusInput
			s.setInput(name)
			return s.onInputChange(), true
		}
	case tea.KeyEnter:
		if s.liveSel < len(res) {
			s.focus = focusInput
			s.push(s.resultMenu(res[s.liveSel]))
			return nil, true
		}
	}
	s.focus = focusInput
	return nil, false
}

func (s *shell) setInput(v string) {
	s.in.SetValue(v)
	s.in.CursorEnd()
	s.suggHidden = false
	s.suggNav = false
	s.focus = focusInput
	s.refreshSuggestions()
	if v == "" {
		s.live = liveState{seq: s.live.seq + 1}
	}
}

func (s *shell) menuOpen() bool { return len(s.sugg) > 0 && !s.suggHidden }

func (s *shell) onInputChange() tea.Cmd {
	s.suggHidden = false
	s.suggNav = false
	s.histPos = len(s.history)
	cmds := []tea.Cmd{s.scheduleLive()}
	if s.refreshSuggestions() {
		cmds = append(cmds, s.ensureDomains())
	}
	return tea.Batch(cmds...)
}

// ---------------------------------------------------------------- below the prompt

func (s *shell) belowPrompt(w int) []string {
	p := s.p
	if s.menuOpen() {
		var lines []string
		lw := 0
		for _, sg := range s.sugg {
			if x := ui.Width(sg.label); x > lw {
				lw = x
			}
		}
		const rows = 8
		first := 0
		if s.suggSel >= rows {
			first = s.suggSel - rows + 1
		}
		for i := first; i < len(s.sugg) && i < first+rows; i++ {
			sg := s.sugg[i]
			label := ui.PadRight(sg.label, lw)
			desc := ui.Truncate(sg.desc, max(0, w-lw-8))
			if i == s.suggSel {
				lines = append(lines, " "+p.BoldFg(ui.Green, "› "+label)+"  "+desc)
			} else {
				lines = append(lines, "   "+p.Fg(ui.NavyTint, label)+"  "+p.Muted(desc))
			}
		}
		if more := len(s.sugg) - first - rows; more > 0 {
			lines = append(lines, "   "+p.Muted("··· "+strconv.Itoa(more)+" more below"))
		}
		hint := "↑↓ choose · tab complete · enter run · esc hide"
		if strings.HasPrefix(s.in.Value(), "/") {
			hint = "↑↓ choose · enter run · tab complete · esc hide"
		}
		return append(lines, s.hint(hint))
	}
	if lines := s.liveView(w); len(lines) > 0 {
		return lines
	}
	if strings.TrimSpace(s.in.Value()) == "" {
		return s.homeView(w)
	}
	return []string{s.hint("enter run · tab complete · esc clear")}
}

// homeView lists the quick actions under an empty prompt.
func (s *shell) homeView(w int) []string {
	p := s.p
	items := s.homeItems()
	lw := 0
	for _, it := range items {
		if x := ui.Width(it.label); x > lw {
			lw = x
		}
	}
	focused := s.focus == focusHome
	var lines []string
	for i, it := range items {
		num := strconv.Itoa(i + 1)
		desc := ui.Truncate(it.desc, max(0, w-lw-12))
		switch {
		case focused && i == s.homeSel:
			lines = append(lines, " "+p.BoldFg(ui.Green, "› "+num+"  "+ui.PadRight(it.label, lw))+"  "+desc)
		case focused:
			lines = append(lines, "   "+p.Fg(ui.Green, num)+"  "+ui.PadRight(it.label, lw)+"  "+p.Muted(desc))
		default:
			lines = append(lines, "   "+p.Muted(num+"  "+ui.PadRight(it.label, lw)+"  "+desc))
		}
	}
	if focused {
		return append(lines, s.hint("↑↓ choose · enter open · 1-"+strconv.Itoa(len(items))+" shortcut · esc back to typing"))
	}
	return append(lines, "  "+p.Fg(ui.Green, "↓")+p.Muted(" choose an action  ·  or type a domain name to search  ·  ")+p.Command("/")+p.Muted(" every command  ·  ")+p.Command("?")+p.Muted(" keys"))
}

// ---------------------------------------------------------------- completion

// refreshSuggestions rebuilds the menu under the prompt. It reports whether
// the menu wants the account's domains (to complete a domain argument).
func (s *shell) refreshSuggestions() bool {
	v := s.in.Value()
	s.sugg, s.suggSel = nil, 0
	wantDomains := false
	switch {
	case strings.HasPrefix(v, "/"):
		name, arg, hasArg := strings.Cut(v, " ")
		if !hasArg {
			for _, c := range slashCommands {
				if strings.HasPrefix(c.name, name) {
					label := c.name
					if c.args != "" {
						label += " " + c.args
					}
					s.sugg = append(s.sugg, suggestion{value: c.name, label: label, desc: c.desc, run: true})
				}
			}
			break
		}
		for _, c := range slashCommands {
			if c.name == name && c.args == "<domain>" {
				wantDomains = true
				s.sugg = s.domainSuggestions(name+" ", strings.TrimSpace(arg), true)
			}
		}
	case strings.TrimSpace(v) != "":
		var domains bool
		s.sugg, domains = s.commandSuggestions(v)
		wantDomains = domains
	}
	if len(s.sugg) > 40 {
		s.sugg = s.sugg[:40]
	}
	return wantDomains
}

func (s *shell) domainSuggestions(head, prefix string, run bool) []suggestion {
	var out []suggestion
	p := strings.ToLower(prefix)
	for _, it := range s.domains.items {
		d, _ := it["domainName"].(string)
		if p != "" && !strings.Contains(strings.ToLower(d), p) {
			continue
		}
		if strings.EqualFold(d, prefix) && !run {
			continue
		}
		out = append(out, suggestion{value: head + d, label: d, desc: ui.Strip(expiryText(s.p, it)), run: run})
	}
	return out
}

// commandSuggestions completes command names from the command tree, and the
// domain argument of commands that take one.
func (s *shell) commandSuggestions(v string) ([]suggestion, bool) {
	trimmed := strings.TrimLeft(v, " ")
	if strings.HasPrefix(trimmed, "dna ") {
		trimmed = strings.TrimPrefix(trimmed, "dna ")
	}
	words := strings.Fields(trimmed)
	if len(words) == 0 {
		return nil, false
	}
	trailing := strings.HasSuffix(trimmed, " ")
	cmd := s.root
	i := 0
	for ; i < len(words); i++ {
		if !trailing && i == len(words)-1 {
			break
		}
		var next *cobra.Command
		for _, c := range cmd.Commands() {
			if c.Name() == words[i] || c.HasAlias(words[i]) {
				next = c
			}
		}
		if next == nil {
			break
		}
		cmd = next
	}
	head := strings.Join(words[:i], " ")
	if head != "" {
		head += " "
	}
	// A command whose first argument is a domain: offer the account's domains.
	if cmd != s.root && !cmd.HasSubCommands() && strings.Contains(cmd.Use, "<domain>") {
		rest := words[i:]
		if len(rest) == 0 || (len(rest) == 1 && !trailing && !strings.HasPrefix(rest[0], "-")) {
			prefix := ""
			if len(rest) == 1 {
				prefix = rest[0]
			}
			return s.domainSuggestions(head, prefix, false), true
		}
		return nil, false
	}
	if i < len(words)-1 || (i == len(words)-1 && trailing) {
		return nil, false // typing arguments or flags
	}
	prefix := ""
	if !trailing && i < len(words) {
		prefix = words[i]
	}
	if cmd == s.root && !s.isCommandPrefix(prefix) {
		return nil, false // a domain name, not a command
	}
	var out []suggestion
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		if !strings.HasPrefix(c.Name(), prefix) || c.Name() == prefix && !c.HasSubCommands() {
			continue
		}
		use := c.Name()
		if u := strings.TrimPrefix(c.Use, c.Name()); u != c.Use {
			use += u
		}
		out = append(out, suggestion{value: head + c.Name() + " ", label: head + use, desc: c.Short})
	}
	return out, false
}

func (s *shell) isCommandPrefix(prefix string) bool {
	if prefix == "" {
		return false
	}
	if domainish.MatchString(prefix) && strings.Contains(prefix, ".") {
		return false
	}
	for _, c := range s.root.Commands() {
		if c.IsAvailableCommand() && strings.HasPrefix(c.Name(), prefix) && c.Name() != "completion" {
			return true
		}
	}
	return false
}
