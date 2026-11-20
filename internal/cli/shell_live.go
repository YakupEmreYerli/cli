package cli

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ui"
)

// Live search: while you type a name, the shell checks it across popular
// extensions (one bulk-search call, debounced so it stays well inside the
// API's search rate limit) and lists availability and prices under the
// prompt. ↓ moves into the list; Enter on a result offers what to do next.

var popularTLDs = []string{"com", "net", "org", "io", "co", "app", "dev", "ai"}

type liveState struct {
	seq     int
	query   string
	results []map[string]any
	err     error
	busy    bool
	partial bool   // only the first name is in; the others are still coming
	note    string // shown after the summary, e.g. when only the first name answered
}

type liveDebounceMsg struct {
	seq   int
	query string
}

type liveResultMsg struct {
	seq     int
	query   string
	results []map[string]any
	err     error
	partial bool // the quick answer for the first name alone
}

// liveQuery returns the name to check live, or "" when the input is a
// command, a slash command or too short.
func (s *shell) liveQuery() string {
	v := strings.TrimSpace(s.in.Value())
	if len(v) < 2 || strings.ContainsAny(v, " /") || !domainish.MatchString(v) {
		return ""
	}
	if !strings.Contains(v, ".") && s.isCommandPrefix(v) {
		return "" // "dom" is more likely the start of "domain"
	}
	return strings.ToLower(strings.TrimSuffix(v, "."))
}

// liveReady reports whether results for the current input are on screen.
func (s *shell) liveReady() bool {
	q := s.liveQuery()
	return q != "" && s.live.query == q && len(s.live.results) > 0
}

func (s *shell) scheduleLive() tea.Cmd {
	q := s.liveQuery()
	s.live.seq++
	if q == "" {
		s.live = liveState{seq: s.live.seq}
		return nil
	}
	seq := s.live.seq
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return liveDebounceMsg{seq: seq, query: q} })
}

func (s *shell) onLiveDebounce(m liveDebounceMsg) tea.Cmd {
	if m.seq != s.live.seq || m.query != s.liveQuery() {
		return nil // the user kept typing
	}
	s.live.busy, s.live.partial, s.live.note = true, false, ""
	c := s.app.child(nil, nil, 0)
	if s.env != "" {
		c.flags.env, c.flags.ote = s.env, false
	}
	names := liveNames(m.query)
	// The first name (the .com, or the exact domain typed) is also checked on
	// its own: one search answers in about a second, the bulk search waits
	// for the slowest registry.
	first := func() tea.Msg {
		client, err := c.client()
		if err != nil {
			return nil // the bulk search reports it
		}
		var res struct {
			Info map[string]any `json:"info"`
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := client.Do(ctx, &dna.Request{Method: "POST", Path: "/api/v1/domains/search", Body: map[string]any{"domainName": names[0]}}, &res); err != nil || res.Info == nil {
			return nil
		}
		if n, _ := res.Info["domainName"].(string); n == "" {
			res.Info["domainName"] = names[0]
		}
		return liveResultMsg{seq: m.seq, query: m.query, results: []map[string]any{res.Info}, partial: true}
	}
	return tea.Batch(s.tick(), first, func() tea.Msg {
		client, err := c.client()
		if err != nil {
			return liveResultMsg{seq: m.seq, query: m.query, err: err}
		}
		body := make([]map[string]any, len(names))
		for i, n := range names {
			body[i] = map[string]any{"domainName": n}
		}
		var res struct {
			Infos []map[string]any `json:"infos"`
		}
		// registries that do not answer can hold a search for close to a
		// minute (OTE takes ~50s for .io, .co and .ai); typing on cancels nothing
		// but makes this result stale, so a long wait costs only the spinner
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_, err = client.Do(ctx, &dna.Request{Method: "POST", Path: "/api/v1/domains/bulk-search", Body: body}, &res)
		return liveResultMsg{seq: m.seq, query: m.query, results: res.Infos, err: err}
	})
}

// liveNames lists the domains checked for a query: the exact domain first
// (when it has an extension), then the label with the popular extensions.
func liveNames(q string) []string {
	label, tld, hasDot := strings.Cut(q, ".")
	var names []string
	if hasDot {
		names = append(names, q)
	}
	for _, t := range popularTLDs {
		if hasDot && t == tld {
			continue
		}
		names = append(names, label+"."+t)
	}
	if len(names) > 8 {
		names = names[:8]
	}
	return names
}

func (s *shell) onLiveResult(m liveResultMsg) {
	if m.seq != s.live.seq {
		return
	}
	if m.partial {
		// show the quick answer only while the full list is still on its way
		if s.live.busy {
			s.live.query, s.live.results, s.live.err, s.live.partial = m.query, m.results, nil, true
		}
		return
	}
	s.live.busy = false
	s.live.query = m.query
	if m.err != nil && s.live.partial && len(s.live.results) > 0 {
		// the others did not answer in time: keep the one that did
		s.live.note = "the other extensions did not answer"
	} else {
		s.live.results, s.live.err = m.results, m.err
	}
	s.live.partial = false
	if s.liveSel >= len(s.live.results) {
		s.liveSel = 0
	}
}

func (l liveState) firstAvailable() string {
	for _, r := range l.results {
		if st, _ := r["status"].(string); strings.EqualFold(st, "AVAILABLE") {
			name, _ := r["domainName"].(string)
			return name
		}
	}
	return ""
}

func (s *shell) liveView(w int) []string {
	p := s.p
	q := s.liveQuery()
	if q == "" {
		return nil
	}
	focused := s.focus == focusLive
	head := "  "
	stale := s.live.query != q
	switch {
	case s.live.busy || stale:
		frame := ui.SpinnerFrames[s.frame%len(ui.SpinnerFrames)]
		what := "checking " + q + " across popular extensions"
		if s.live.partial && !stale {
			what = "checking the other extensions"
		}
		head += p.Fg(ui.Green, frame) + " " + p.Shimmer(what, s.frame)
		if len(s.live.results) == 0 || stale {
			return []string{head}
		}
	case s.live.err != nil:
		return []string{head + p.Fg(ui.Amber, "▲ "+shortErr(s.live.err)), s.hint("enter checks " + searchTarget(q) + " on its own")}
	default:
		free := 0
		for _, r := range s.live.results {
			if st, _ := r["status"].(string); strings.EqualFold(st, "AVAILABLE") {
				free++
			}
		}
		head += p.Bold(q) + p.Muted(" · ") + p.Fg(ui.Green, strconv.Itoa(free)+" free") + p.Muted(" of "+strconv.Itoa(len(s.live.results)))
		if s.live.note != "" {
			head += p.Muted(" · " + s.live.note)
		}
	}
	cw := 0
	for _, r := range s.live.results {
		name, _ := r["domainName"].(string)
		if ui.Width(name) > cw {
			cw = ui.Width(name)
		}
	}
	lines := []string{head}
	for i, r := range s.live.results {
		name, _ := r["domainName"].(string)
		st, _ := r["status"].(string)
		selected := focused && i == s.liveSel
		var mark, info string
		switch strings.ToUpper(st) {
		case "AVAILABLE":
			mark = p.BoldFg(ui.Green, "✓")
			if price := priceOf(r); price != "" {
				info = p.Fg(ui.Green, ui.PadLeft(price, 13)) + p.Muted(" / year")
				if prem, _ := r["isPremium"].(bool); prem {
					info += "  " + p.Fg(ui.Amber, "premium")
				}
			} else {
				info = p.Fg(ui.Green, "available")
			}
		case "NOTAVAILABLE":
			mark = p.Fg(ui.Red, "✗")
			info = p.Muted(ui.PadLeft("taken", 13))
		case "UNSUPPORTEDTLD":
			mark = p.Muted("–")
			info = p.Muted(ui.PadLeft("not offered", 13))
		default:
			// ERROR or NONE: the registry did not answer; not the same as taken
			mark = p.Fg(ui.Amber, "?")
			info = p.Muted(ui.PadLeft("no answer", 13))
		}
		label := ui.PadRight(name, cw)
		if selected {
			lines = append(lines, " "+p.BoldFg(ui.Green, "›")+" "+mark+" "+p.BoldFg(ui.Green, label)+"  "+info)
		} else if strings.EqualFold(st, "AVAILABLE") {
			lines = append(lines, "   "+mark+" "+p.Bold(label)+"  "+info)
		} else {
			lines = append(lines, "   "+mark+" "+p.Muted(label)+"  "+info)
		}
	}
	if focused {
		lines = append(lines, s.hint("↑↓ choose · enter what next (register, transfer, …) · tab put it in the prompt · esc back"))
	} else {
		lines = append(lines, "  "+p.Fg(ui.Green, "↓")+p.Muted(" pick a result to register it  ·  enter checks "+searchTarget(q)+"  ·  tab takes the first free one"))
	}
	return lines
}
