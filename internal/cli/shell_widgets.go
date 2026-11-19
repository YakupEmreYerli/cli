package cli

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/domainreseller/cli/internal/ui"
)

// overlay is a screen shown instead of the prompt: a menu, a form, the
// domain browser or a confirmation box. They stack; Esc always goes back one
// level.
type overlay interface {
	view(s *shell, w int) []string
	key(s *shell, k tea.KeyMsg) tea.Cmd
}

func (s *shell) push(o overlay) { s.stack = append(s.stack, o) }

func (s *shell) pop() {
	if n := len(s.stack); n > 0 {
		s.stack = s.stack[:n-1]
	}
}

func (s *shell) top() overlay {
	if n := len(s.stack); n > 0 {
		return s.stack[n-1]
	}
	return nil
}

// unwind closes the menus and forms on top (they did their job), keeping
// longer-lived screens such as the domain browser.
func (s *shell) unwind() {
	for {
		switch s.top().(type) {
		case *menu, *form:
			s.pop()
		default:
			return
		}
	}
}

func (s *shell) hint(text string) string { return "  " + s.p.Muted(text) }

// ---------------------------------------------------------------- menu

type menuItem struct {
	label string
	desc  string
	key   string // one-key shortcut ("" none)
	act   func(s *shell) tea.Cmd
	opens bool // opens another screen on top; this menu stays underneath
	keep  bool // always listed, whatever the filter
}

// menu is a list to choose from: ↑↓ and Enter, a shortcut key, or (when
// filterable) typing to narrow it down.
type menu struct {
	title      string
	items      []menuItem
	build      func(s *shell, filter string) []menuItem // dynamic items
	filterable bool
	filter     string
	sel        int
	top        int
	loading    func(s *shell) bool
	empty      string
	footer     string
}

func (m *menu) visible(s *shell) []menuItem {
	items := m.items
	if m.build != nil {
		items = m.build(s, m.filter)
	}
	if m.filter == "" {
		return items
	}
	f := strings.ToLower(m.filter)
	var out []menuItem
	for _, it := range items {
		if it.keep || strings.Contains(strings.ToLower(it.label), f) {
			out = append(out, it)
		}
	}
	return out
}

func (m *menu) key(s *shell, k tea.KeyMsg) tea.Cmd {
	items := m.visible(s)
	switch k.Type {
	case tea.KeyEsc:
		if m.filter != "" {
			m.filter, m.sel = "", 0
			return nil
		}
		s.pop()
		return nil
	case tea.KeyUp, tea.KeyShiftTab:
		if m.sel > 0 {
			m.sel--
		}
		return nil
	case tea.KeyDown, tea.KeyTab:
		if m.sel < len(items)-1 {
			m.sel++
		}
		return nil
	case tea.KeyPgUp, tea.KeyHome:
		m.sel = 0
		return nil
	case tea.KeyPgDown, tea.KeyEnd:
		m.sel = len(items) - 1
		return nil
	case tea.KeyEnter:
		if m.sel >= 0 && m.sel < len(items) {
			return s.choose(items[m.sel])
		}
		return nil
	case tea.KeyBackspace:
		if m.filterable && m.filter != "" {
			r := []rune(m.filter)
			m.filter, m.sel = string(r[:len(r)-1]), 0
		}
		return nil
	case tea.KeyRunes, tea.KeySpace:
		r := string(k.Runes)
		if k.Type == tea.KeySpace {
			r = " "
		}
		if m.filterable {
			m.filter += r
			m.sel = 0
			return nil
		}
		for _, it := range items {
			if it.key != "" && strings.EqualFold(it.key, r) {
				return s.choose(it)
			}
		}
		switch r {
		case "k":
			if m.sel > 0 {
				m.sel--
			}
		case "j":
			if m.sel < len(items)-1 {
				m.sel++
			}
		case "q":
			s.pop()
		}
	}
	return nil
}

func (s *shell) choose(it menuItem) tea.Cmd {
	if !it.opens {
		s.unwind()
	}
	return it.act(s)
}

func (m *menu) view(s *shell, w int) []string {
	p := s.p
	items := m.visible(s)
	if m.sel >= len(items) {
		m.sel = max(0, len(items)-1)
	}
	var lines []string
	if m.filterable {
		search := p.Fg(ui.Green, "⌕ ") + p.Bold(m.filter) + p.Fg(ui.Green, "▏")
		if m.filter == "" {
			search += p.Muted("type to search")
		}
		lines = append(lines, search, "")
	}
	if m.loading != nil && m.loading(s) {
		frame := ui.SpinnerFrames[s.frame%len(ui.SpinnerFrames)]
		lines = append(lines, p.Fg(ui.Green, frame)+" "+p.Shimmer("Fetching your domains…", s.frame))
	}
	rows := 10
	if len(items) < rows {
		rows = len(items)
	}
	if m.sel < m.top {
		m.top = m.sel
	}
	if m.sel >= m.top+rows {
		m.top = m.sel - rows + 1
	}
	lw := 0
	for _, it := range items {
		if x := ui.Width(it.label); x > lw {
			lw = x
		}
	}
	inner := min(w, 100) - 4
	// on a narrow terminal one long label must not push every description off
	if c := max(12, inner*2/5); lw > c {
		lw = c
	}
	hasKeys, kw := false, 0
	for _, it := range items {
		hasKeys = hasKeys || it.key != ""
	}
	if hasKeys {
		kw = 3
	}
	for i := m.top; i < m.top+rows && i < len(items); i++ {
		it := items[i]
		key := ""
		if hasKeys {
			key = "   "
		}
		if it.key != "" {
			key = p.Fg(ui.Green, it.key) + "  "
		}
		desc := ui.Truncate(it.desc, max(0, inner-lw-kw-5))
		label := ui.PadRight(ui.Truncate(it.label, lw), lw)
		if i == m.sel {
			lines = append(lines, p.BoldFg(ui.Green, "›")+" "+key+p.BoldFg(ui.Green, label)+"  "+desc)
		} else {
			lines = append(lines, "  "+key+label+"  "+p.Muted(desc))
		}
	}
	if len(items) == 0 && (m.loading == nil || !m.loading(s)) {
		lines = append(lines, p.Muted(m.empty))
	}
	if len(items) > rows {
		more := "  ··· " + strconv.Itoa(len(items)-rows) + " more, keep scrolling"
		if m.filterable {
			more += " or type to search"
		}
		lines = append(lines, p.Muted(more))
	}
	out := p.Box(lines, min(w, 100), ui.GreenDeep, m.title)
	footer := m.footer
	if footer == "" {
		footer = "↑↓ choose · enter select · esc back"
		if m.filterable {
			footer += " · type to search"
		}
	}
	return append(out, s.hint(footer))
}

// ---------------------------------------------------------------- form

type formField struct {
	name     string
	label    string
	hint     string
	text     bool
	in       textinput.Model
	options  []string
	optLabel func(v string) string
	choice   int
	required bool
	// pick, on a choice field, opens a full list on Enter (←→ still switch).
	pick func(s *shell, f *formField) tea.Cmd
}

func textField(name, label, value, placeholder, hint string, required bool) *formField {
	return &formField{name: name, label: label, hint: hint, text: true, in: newTextInput(value, placeholder), required: required}
}

func choiceField(name, label string, options []string, def int, hint string, optLabel func(string) string) *formField {
	return &formField{name: name, label: label, hint: hint, options: options, choice: def, optLabel: optLabel}
}

func (f *formField) value() string {
	if f.text {
		return strings.TrimSpace(f.in.Value())
	}
	return f.options[f.choice]
}

// form is a short sequence of fields ending in an action button. Tab, ↓ and
// Enter move on; ←→ changes a choice; Enter on the button submits.
type form struct {
	title  string
	intro  string
	fields []*formField
	focus  int // len(fields) is the button
	action string
	note   func(f *form) string
	err    string
	submit func(s *shell, v map[string]string) tea.Cmd
}

func (f *form) init() *form {
	f.setFocus(0)
	return f
}

func (f *form) setFocus(i int) {
	if i < 0 {
		i = 0
	}
	if i > len(f.fields) {
		i = len(f.fields)
	}
	f.focus = i
	for j, fld := range f.fields {
		if fld.text {
			if j == i {
				fld.in.Focus()
				fld.in.CursorEnd()
			} else {
				fld.in.Blur()
			}
		}
	}
}

func (f *form) values() map[string]string {
	v := map[string]string{}
	for _, fld := range f.fields {
		v[fld.name] = fld.value()
	}
	return v
}

func (f *form) update(msg tea.Msg) tea.Cmd {
	if f.focus < len(f.fields) && f.fields[f.focus].text {
		var cmd tea.Cmd
		f.fields[f.focus].in, cmd = f.fields[f.focus].in.Update(msg)
		return cmd
	}
	return nil
}

func (f *form) key(s *shell, k tea.KeyMsg) tea.Cmd {
	f.err = ""
	var cur *formField
	if f.focus < len(f.fields) {
		cur = f.fields[f.focus]
	}
	switch k.String() {
	case "esc":
		s.pop()
		return nil
	case "tab", "down":
		f.setFocus(f.focus + 1)
		return nil
	case "shift+tab", "up":
		f.setFocus(f.focus - 1)
		return nil
	case "enter":
		if cur != nil && cur.pick != nil {
			return cur.pick(s, cur)
		}
		if cur != nil && f.focus < len(f.fields)-1 {
			f.setFocus(f.focus + 1)
			return nil
		}
		for _, fld := range f.fields {
			if fld.required && fld.value() == "" {
				f.err = fld.label + " is required"
				f.setFocus(indexOf(f.fields, fld))
				return nil
			}
		}
		s.unwind()
		return f.submit(s, f.values())
	case "left", "right":
		if cur != nil && !cur.text {
			n := len(cur.options)
			if k.String() == "left" {
				cur.choice = (cur.choice + n - 1) % n
			} else {
				cur.choice = (cur.choice + 1) % n
			}
			return nil
		}
	}
	if cur != nil && cur.text {
		var cmd tea.Cmd
		cur.in, cmd = cur.in.Update(k)
		return cmd
	}
	return nil
}

func indexOf(fields []*formField, f *formField) int {
	for i, x := range fields {
		if x == f {
			return i
		}
	}
	return 0
}

func (f *form) view(s *shell, w int) []string {
	p := s.p
	bw := min(w, 100)
	inner := bw - 4
	lw := 0
	for _, fld := range f.fields {
		if x := ui.Width(fld.label); x > lw {
			lw = x
		}
	}
	var lines []string
	if f.intro != "" {
		for _, l := range wrapCode(f.intro, inner) {
			lines = append(lines, p.Muted(l))
		}
		lines = append(lines, "")
	}
	for i, fld := range f.fields {
		focused := i == f.focus
		mark := "  "
		label := ui.PadRight(fld.label, lw)
		if focused {
			mark = p.BoldFg(ui.Green, "›") + " "
			label = p.Bold(label)
		} else {
			label = p.Muted(label)
		}
		var val string
		switch {
		case fld.text && focused:
			fld.in.Width = max(10, inner-lw-6)
			val = fld.in.View()
		case fld.text:
			val = fld.in.Value()
			if val == "" {
				val = p.Muted(fld.in.Placeholder)
			}
		default:
			v := fld.options[fld.choice]
			if fld.optLabel != nil {
				v = fld.optLabel(v)
			}
			if focused {
				val = p.Fg(ui.Green, "‹ ") + p.Bold(v) + p.Fg(ui.Green, " ›")
			} else {
				val = v
			}
		}
		lines = append(lines, mark+label+"   "+val)
		if focused && fld.hint != "" {
			lines = append(lines, "  "+strings.Repeat(" ", lw)+"   "+p.Muted(ui.Truncate(fld.hint, max(10, inner-lw-6))))
		}
	}
	lines = append(lines, "")
	if f.note != nil {
		if n := f.note(f); n != "" {
			lines = append(lines, n, "")
		}
	}
	if f.err != "" {
		lines = append(lines, p.Fg(ui.Red, "✗ "+f.err), "")
	}
	button := "[ " + f.action + " ]"
	if f.focus == len(f.fields) {
		lines = append(lines, p.BoldFg(ui.Green, "›")+" "+p.Badge(ui.Green, f.action)+p.Muted("  press enter"))
	} else {
		lines = append(lines, "  "+p.Muted(button))
	}
	out := p.Box(lines, bw, ui.GreenDeep, f.title)
	return append(out, s.hint("tab / ↓ next field · ←→ change a choice · enter continue · esc back"))
}
