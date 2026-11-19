package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/domainreseller/cli/internal/config"
	"github.com/domainreseller/cli/internal/ui"
)

// Contacts in the shell: the registration form offers the saved contacts
// (the default first), "New contact…" opens a short form right there and
// "Another handle…" takes one typed in. With an empty contact book the shell
// reads the contacts of a few domains in the background, so existing
// handles show up without being looked up by hand.

// discoverLimit is how many domains the shell reads to find contacts.
const discoverLimit = "5"

// The contact field's two actions next to the saved handles.
const (
	contactNew   = "+new"
	contactOther = "+other"
)

// childApp is an App for one step of the shell, in the session's environment.
func (s *shell) childApp() *App {
	c := s.app.child(nil, nil, 0)
	if s.env != "" {
		c.flags.env, c.flags.ote = s.env, false
	}
	return c
}

// contacts reads the contact book fresh (commands the shell ran may have
// changed it).
func (s *shell) contacts() *contactBook {
	b, err := s.childApp().contactBook()
	if err != nil || b == nil {
		return &contactBook{}
	}
	return b
}

// contactOptions are the values of the contact field: saved handles, the
// handle the shell used before contact books existed, then the two actions.
func (s *shell) contactOptions(b *contactBook) []string {
	var opts []string
	for _, c := range b.contacts {
		opts = append(opts, c.Handle)
	}
	if h := s.prefs.Handle; h != "" && b.find(h) == nil {
		opts = append(opts, h)
	}
	return append(opts, contactNew, contactOther)
}

func (s *shell) contactField(b *contactBook) *formField {
	f := choiceField("contact", "Contact", nil, 0, "", nil)
	s.refreshContactField(f, b, false)
	f.pick = func(s *shell, f *formField) tea.Cmd {
		s.push(s.contactPicker(f))
		return nil
	}
	return f
}

func (s *shell) refreshContactField(f *formField, b *contactBook, searching bool) {
	cur := ""
	if f.choice < len(f.options) {
		cur = f.options[f.choice]
	}
	f.options = s.contactOptions(b)
	f.choice = 0
	for i, o := range f.options {
		if o == cur && cur != contactNew {
			f.choice = i
		}
	}
	f.optLabel = func(v string) string {
		switch v {
		case contactNew:
			return "New contact…"
		case contactOther:
			return "Another handle…"
		}
		if c := b.find(v); c != nil && c.Name != "" {
			v += "  " + c.Name
		}
		return v
	}
	switch {
	case searching:
		f.hint = "looking for the contacts on your domains…"
	case len(b.contacts) == 0 && s.prefs.Handle == "":
		f.hint = "no saved contacts yet: New contact… creates one here"
	default:
		f.hint = "used for all four roles · ←→ switch · enter shows all"
	}
}

// contactPicker lists every saved contact; typing narrows the list.
func (s *shell) contactPicker(f *formField) *menu {
	b := s.contacts()
	set := func(v string) func(s *shell) tea.Cmd {
		return func(s *shell) tea.Cmd {
			for i, o := range f.options {
				if o == v {
					f.choice = i
				}
			}
			s.pop()
			return nil
		}
	}
	var items []menuItem
	for _, o := range f.options {
		it := menuItem{label: f.optLabel(o), opens: true, act: set(o)}
		switch o {
		case contactNew:
			it.desc, it.keep = "a short form; it gets a handle right away", true
		case contactOther:
			it.desc, it.keep = "type a handle you already have", true
		default:
			if c := b.find(o); c != nil {
				it.desc = strings.TrimSpace(c.Email + "  " + c.Country)
				if strings.EqualFold(o, b.def) {
					it.desc = "default · " + it.desc
				}
			}
		}
		items = append(items, it)
	}
	return &menu{
		title:      s.p.Bold("Contact") + s.p.Muted(" · used as registrant, admin, tech and billing"),
		items:      items,
		filterable: true,
		footer:     "↑↓ choose · enter use it · type to search · esc back",
	}
}

// reopen shows a form again after a failed step, on top of the form it was
// opened from (so Esc still goes back there).
func reopen(parent, f *form) func(s *shell) {
	return func(s *shell) {
		if parent != nil {
			s.push(parent)
		}
		s.push(f)
	}
}

// contactForm creates a contact in a few fields, then hands its handle to
// next (nil just reports it). parent is the form it was opened from.
func (s *shell) contactForm(parent *form, next func(s *shell, handle string) tea.Cmd) *form {
	f := &form{
		title: s.p.Bold("New contact"),
		intro: "Used as the registrant, admin, tech and billing contact. dna saves the handle, so you never type it again.",
		fields: []*formField{
			textField("first", "First name", "", "", "", true),
			textField("last", "Last name", "", "", "", true),
			textField("email", "E-mail", "", "name@example.com", "", true),
			textField("phone", "Phone", "", "+90 555 123 4567", "with the country code; spaces are fine", true),
			textField("country", "Country", "", "TR", "2 letters; empty takes it from the phone's country code", false),
			textField("city", "City", "", "", "", true),
			textField("state", "State / province", "", "", "empty uses the city", false),
			textField("address", "Address", "", "street and number", "", true),
			textField("postal", "Postal code", "", "", "", true),
			textField("company", "Company", "", "optional", "empty uses the name", false),
		},
		action: "Create the contact",
	}
	f.submit = func(s *shell, v map[string]string) tea.Cmd {
		state := v["state"]
		if state == "" {
			state = v["city"]
		}
		words := []string{"contact", "create", "--first-name", v["first"], "--last-name", v["last"], "--email", v["email"],
			"--phone", v["phone"], "--city", v["city"], "--state", state, "--address", v["address"], "--postal-code", v["postal"]}
		if v["country"] != "" {
			words = append(words, "--country", v["country"])
		}
		if v["company"] != "" {
			words = append(words, "--company-name", v["company"])
		}
		return s.contactStep(words, "Creating the contact", true, next, reopen(parent, f))
	}
	return f.init()
}

// handleForm takes a handle the person already has, checks it with the API
// (and saves it), then hands it to next.
func (s *shell) handleForm(parent *form, next func(s *shell, handle string) tea.Cmd) *form {
	f := &form{
		title:  s.p.Bold("Use a contact handle"),
		fields: []*formField{textField("handle", "Handle", "", "e.g. D-123456789", "dna checks it and saves it with the contact's name", true)},
		action: "Check and use it",
	}
	f.submit = func(s *shell, v map[string]string) tea.Cmd {
		return s.contactStep([]string{"contact", "get", v["handle"]}, "Checking "+v["handle"], false, next, reopen(parent, f))
	}
	return f.init()
}

type contactStepMsg struct {
	handle  string
	out     string
	code    int
	created bool
	next    func(s *shell, handle string) tea.Cmd
	retry   func(s *shell)
}

// contactStep runs a contact command for its handle (JSON), turning API
// errors into the usual human message.
func (s *shell) contactStep(words []string, label string, created bool, next func(*shell, string) tea.Cmd, retry func(*shell)) tea.Cmd {
	app, args := s.app, s.args(append(append([]string{}, words...), "-o", "json"))
	s.run = runState{active: true, label: label, start: time.Now(), cancel: func() {}}
	return tea.Batch(s.tick(), func() tea.Msg {
		var out, errb bytes.Buffer
		c := app.child(&out, &errb, 0)
		code := c.Run(context.Background(), args)
		m := contactStepMsg{code: code, out: strings.TrimRight(errb.String(), "\n"), created: created, next: next, retry: retry}
		var e struct {
			Error errorPayload `json:"error"`
		}
		if code != 0 && json.Unmarshal([]byte(m.out), &e) == nil && e.Error.Kind != "" {
			m.out = strings.TrimRight(c.humanError(e.Error), "\n")
		}
		var r struct {
			Handle  string         `json:"handle"`
			Contact map[string]any `json:"contact"`
		}
		if code == 0 && json.Unmarshal(out.Bytes(), &r) == nil {
			m.handle = r.Handle
			if m.handle == "" && r.Contact != nil {
				m.handle = contactFrom(r.Contact).Handle
			}
			if m.handle == "" && len(words) == 3 {
				m.handle = words[2]
			}
		}
		return m
	})
}

func (s *shell) onContactStep(m contactStepMsg) tea.Cmd {
	s.run = runState{}
	if m.code != 0 || m.handle == "" {
		if m.retry != nil {
			m.retry(s)
		}
		if m.out == "" {
			m.out = "  " + s.p.Fg(ui.Red, "✗") + " No contact handle came back; nothing else was sent."
		}
		return tea.Println(m.out)
	}
	line := "  " + s.p.BoldFg(ui.Green, "✓") + " Contact " + s.p.Bold(m.handle)
	if m.created {
		line += " created"
		if strings.EqualFold(s.contacts().def, m.handle) {
			line += " and saved as your default"
		} else {
			line += " and saved to your contacts"
		}
	} else {
		line += " checked and saved to your contacts"
	}
	cmds := []tea.Cmd{tea.Println(line)}
	if m.next != nil {
		cmds = append(cmds, m.next(s, m.handle))
	}
	return tea.Sequence(cmds...)
}

type contactsFoundMsg struct {
	contacts []config.Contact
}

// discoverContacts reads the contacts of a few domains in the background
// (only when the contact book is empty).
func (s *shell) discoverContacts() tea.Cmd {
	app, args := s.app, s.args([]string{"contact", "list", "--scan", "--limit", discoverLimit, "-o", "json"})
	return func() tea.Msg {
		var out bytes.Buffer
		c := app.child(&out, nil, 0)
		c.StdoutTTY = false
		var r struct {
			Contacts []config.Contact `json:"contacts"`
		}
		if c.Run(context.Background(), args) == 0 {
			_ = json.Unmarshal(out.Bytes(), &r)
		}
		return contactsFoundMsg{contacts: r.Contacts}
	}
}

func (s *shell) onContactsFound(m contactsFoundMsg) {
	b := s.contacts()
	for _, o := range s.stack {
		f, ok := o.(*form)
		if !ok {
			continue
		}
		for _, fld := range f.fields {
			if fld.name == "contact" && fld.pick != nil {
				s.refreshContactField(fld, b, false)
			}
		}
	}
}

// openRegister shows the registration form, looking for contacts in the
// background when none are saved yet.
func (s *shell) openRegister(d, price string) tea.Cmd {
	b := s.contacts()
	f := s.registerForm(d, price, b)
	s.push(f)
	if len(b.contacts) > 0 || s.prefs.Handle != "" {
		return nil
	}
	for _, fld := range f.fields {
		if fld.name == "contact" {
			s.refreshContactField(fld, b, true)
		}
	}
	return s.discoverContacts()
}
