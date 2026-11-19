package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ui"
)

// ---------------------------------------------------------------- domain cache

const (
	domainsNone = iota
	domainsLoading
	domainsLoaded
)

// domainCache holds the account's domains for the browser, the domain
// picker and argument completion. It loads on first use only.
type domainCache struct {
	state int
	items []map[string]any
	err   string
}

type domainsMsg struct {
	items []map[string]any
	err   string
}

func (s *shell) ensureDomains() tea.Cmd {
	if s.domains.state != domainsNone {
		return nil
	}
	s.domains.state = domainsLoading
	app, args := s.app, s.args([]string{"domain", "list", "--all", "-o", "json"})
	return tea.Batch(s.tick(), func() tea.Msg {
		var out, errb bytes.Buffer
		c := app.child(&out, &errb, 0)
		c.StdoutTTY = false
		if code := c.Run(context.Background(), args); code != 0 {
			return domainsMsg{err: strings.TrimRight(errb.String(), "\n")}
		}
		var res struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			return domainsMsg{err: "  " + err.Error()}
		}
		return domainsMsg{items: res.Items}
	})
}

func (s *shell) onDomains(m domainsMsg) tea.Cmd {
	if m.err != "" {
		s.domains = domainCache{state: domainsNone, err: m.err}
		// close the screens that were waiting for the list
		for {
			switch s.top().(type) {
			case *browser, *menu:
				s.pop()
				continue
			}
			break
		}
		return tea.Println(m.err)
	}
	s.domains = domainCache{state: domainsLoaded, items: m.items}
	return nil
}

// invalidateAfter reloads the domain list after a change (renewal,
// registration, nameservers) so expiry dates and settings are current.
func invalidateAfter(s *shell, code int) tea.Cmd {
	if code != 0 || s.domains.state != domainsLoaded {
		return nil
	}
	s.domains.state = domainsNone
	return s.ensureDomains()
}

// toggleAfter flips a boolean of a cached domain once the change went
// through, so menus and the browser are right without reloading.
func toggleAfter(name, key string) func(s *shell, code int) tea.Cmd {
	return func(s *shell, code int) tea.Cmd {
		if code != 0 {
			return nil
		}
		if it := s.cachedDomain(name); it != nil {
			on, _ := it[key].(bool)
			it[key] = !on
		}
		return nil
	}
}

// ---------------------------------------------------------------- browser

// browser lists the account's domains: ↑↓ to move, type to search, Enter
// for the domain's menu, or a letter for a direct action.
type browser struct {
	filter string
	sel    int
	top    int
}

func (s *shell) openBrowser() tea.Cmd {
	s.push(&browser{})
	return s.ensureDomains()
}

func (b *browser) visible(s *shell) []map[string]any {
	if b.filter == "" {
		return s.domains.items
	}
	var out []map[string]any
	f := strings.ToLower(b.filter)
	for _, it := range s.domains.items {
		if name, _ := it["domainName"].(string); strings.Contains(strings.ToLower(name), f) {
			out = append(out, it)
		}
	}
	return out
}

func (b *browser) key(s *shell, k tea.KeyMsg) tea.Cmd {
	items := b.visible(s)
	n := len(items)
	switch k.Type {
	case tea.KeyEsc:
		if b.filter != "" {
			b.filter, b.sel, b.top = "", 0, 0
			return nil
		}
		s.pop()
		return nil
	case tea.KeyUp:
		if b.sel > 0 {
			b.sel--
		}
		return nil
	case tea.KeyDown:
		if b.sel < n-1 {
			b.sel++
		}
		return nil
	case tea.KeyPgUp:
		b.sel = max(0, b.sel-10)
		return nil
	case tea.KeyPgDown:
		b.sel = max(0, min(n-1, b.sel+10))
		return nil
	case tea.KeyHome:
		b.sel = 0
		return nil
	case tea.KeyEnd:
		b.sel = max(0, n-1)
		return nil
	case tea.KeyBackspace:
		if r := []rune(b.filter); len(r) > 0 {
			b.filter, b.sel, b.top = string(r[:len(r)-1]), 0, 0
		}
		return nil
	case tea.KeyCtrlR:
		s.domains.state = domainsNone
		return s.ensureDomains()
	case tea.KeyEnter:
		if b.sel < n {
			name, _ := items[b.sel]["domainName"].(string)
			s.push(s.domainMenu(name))
		}
		return nil
	case tea.KeyRunes:
		// typing always searches; actions are one Enter away
		b.filter += string(k.Runes)
		b.sel, b.top = 0, 0
	}
	return nil
}

func (b *browser) view(s *shell, w int) []string {
	p := s.p
	items := b.visible(s)
	var lines []string
	search := p.Fg(ui.Green, "⌕ ") + p.Bold(b.filter) + p.Fg(ui.Green, "▏")
	if b.filter == "" {
		search += p.Muted("type to search, enter to manage the selected domain")
	}
	lines = append(lines, search, "")
	if s.domains.state == domainsLoading && len(s.domains.items) == 0 {
		frame := ui.SpinnerFrames[s.frame%len(ui.SpinnerFrames)]
		lines = append(lines, p.Fg(ui.Green, frame)+" "+p.Shimmer("Fetching your domains…", s.frame))
	}
	rows := 12
	if len(items) < rows {
		rows = len(items)
	}
	if b.sel >= len(items) {
		b.sel = max(0, len(items)-1)
	}
	if b.sel < b.top {
		b.top = b.sel
	}
	if b.sel >= b.top+rows {
		b.top = b.sel - rows + 1
	}
	nameW := 10
	for _, it := range items {
		if n, _ := it["domainName"].(string); ui.Width(n) > nameW {
			nameW = ui.Width(n)
		}
	}
	// Columns: "› ● " name "  " date+days (19) "   " lock (6) "  " privacy (9).
	// Narrow terminals drop the privacy column so names stay readable.
	bw := min(w, 110)
	showPrivacy := bw >= 86
	fixed := 4 + 4 + 2 + 19 + 3 + 6
	if showPrivacy {
		fixed += 2 + 9
	}
	nameW = max(10, min(nameW, bw-fixed))
	now := time.Now()
	for i := b.top; i < b.top+rows && i < len(items); i++ {
		it := items[i]
		name, _ := it["domainName"].(string)
		status := domainStatusName(it["status"])
		dot := p.Muted("●")
		if t := ui.ToneOf(status); t != ui.Neutral {
			dot = p.Fg(t.Color(), "●")
		}
		exp := ""
		if e, _ := it["expirationDate"].(string); len(e) >= 10 {
			n := daysLeft(e[:10], now)
			if rd, ok := it["remainingDay"].(float64); ok {
				n = int(rd)
			}
			rel := fmt.Sprintf("in %dd", n)
			if n < 0 {
				rel = fmt.Sprintf("%dd ago", -n)
			}
			relS := p.Muted(ui.PadLeft(rel, 8))
			switch {
			case n < 0:
				relS = p.Fg(ui.Red, ui.PadLeft(rel, 8))
			case n <= 30:
				relS = p.Fg(ui.Amber, ui.PadLeft(rel, 8))
			}
			exp = e[:10] + " " + relS
		}
		flag := func(key, label string) string {
			if on, _ := it[key].(bool); on {
				return p.Fg(ui.Green, "✓ "+label)
			}
			return p.Muted("· " + label)
		}
		shown := ui.PadRight(ui.Truncate(name, nameW), nameW)
		rest := "  " + exp + "   " + flag("lockStatus", "lock")
		if showPrivacy {
			rest += "  " + flag("privacyProtectionStatus", "privacy")
		}
		if i == b.sel {
			lines = append(lines, p.BoldFg(ui.Green, "›")+" "+dot+" "+p.BoldFg(ui.Green, shown)+rest)
		} else {
			lines = append(lines, "  "+dot+" "+shown+rest)
		}
	}
	if len(items) == 0 && s.domains.state == domainsLoaded {
		if b.filter != "" {
			lines = append(lines, p.Muted("No domain matches “"+b.filter+"”."))
		} else {
			lines = append(lines, p.Muted("No domains in this account yet. Type a name at the prompt to find one to register."))
		}
	}
	if len(items) > rows {
		lines = append(lines, p.Muted(fmt.Sprintf("  ··· %d of %d shown, scroll with ↑↓", rows, len(items))))
	}
	title := p.BoldFg(ui.Green, "Your domains") + p.Muted(fmt.Sprintf(" · %d", len(s.domains.items)))
	if s.domains.state == domainsLoading && len(s.domains.items) > 0 {
		title += p.Muted(" · refreshing…")
	}
	out := p.Box(lines, min(w, 110), ui.GreenDeep, title)
	return append(out, s.hint("↑↓ choose · enter all actions · type to search · ctrl+r reload · esc back"))
}

func domainStatusName(v any) string {
	switch x := v.(type) {
	case float64:
		if e, ok := dna.LookupEnum("DomainStatus"); ok {
			if n := e.NameOf(int(x)); n != "" {
				return n
			}
		}
		return fmt.Sprint(x)
	case string:
		return x
	}
	return ""
}

func daysLeft(date string, now time.Time) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(t.Sub(today).Hours() / 24)
}
