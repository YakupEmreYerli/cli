package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/domainreseller/cli/internal/ui"
)

// platformNameservers are the Domain Name API nameservers (DNS records
// managed with `dna dns` only resolve when a domain uses them).
const platformNameservers = "tr.apiname.com, eu.apiname.com"

// ---------------------------------------------------------------- quick actions

func (s *shell) homeItems() []menuItem {
	env := menuItem{label: "Switch to OTE (test)", desc: "try everything without real money", act: func(s *shell) tea.Cmd { return s.switchEnv("ote") }}
	if strings.EqualFold(s.currentEnv(), "ote") {
		env = menuItem{label: "Switch to production", desc: "you are on OTE, the test environment", act: func(s *shell) tea.Cmd { return s.switchEnv("production") }}
	}
	return []menuItem{
		{label: "My domains", desc: "browse, renew, lock, DNS, auth codes", act: func(s *shell) tea.Cmd { return s.openBrowser() }},
		{label: "Register a domain", desc: "check it, see the price, register it", act: func(s *shell) tea.Cmd { return s.registerStart() }},
		{label: "DNS records", desc: "pick a domain, see and add records", act: func(s *shell) tea.Cmd {
			return s.pickDomain("DNS records of…", func(s *shell, d string) tea.Cmd { s.push(s.dnsMenu(d)); return nil }, true)
		}},
		{label: "Balance", desc: "balance and the latest transactions", act: func(s *shell) tea.Cmd {
			return tea.Sequence(s.start([]string{"account", "get"}, "Counting your balance", runOpts{}),
				s.start([]string{"account", "transactions", "--max-result-count", "10"}, "Reading transactions", runOpts{}))
		}},
		env,
		{label: "Connect an AI agent", desc: "Claude Code, ChatGPT, Cursor and 18 more, with ready-made prompts", opens: true, act: func(s *shell) tea.Cmd {
			return s.agentPicker()
		}},
		{label: "Keys and help", desc: "everything you can type and press", act: func(s *shell) tea.Cmd { return tea.Println(s.helpText()) }},
	}
}

// ---------------------------------------------------------------- domain menu

func (s *shell) cachedDomain(name string) map[string]any {
	for _, it := range s.domains.items {
		if n, _ := it["domainName"].(string); strings.EqualFold(n, name) {
			return it
		}
	}
	return nil
}

func expiryText(p ui.Painter, it map[string]any) string {
	e, _ := it["expirationDate"].(string)
	if len(e) < 10 {
		return ""
	}
	n := daysLeft(e[:10], time.Now())
	if rd, ok := it["remainingDay"].(float64); ok {
		n = int(rd)
	}
	switch {
	case n < 0:
		return p.Fg(ui.Red, fmt.Sprintf("expired %d days ago", -n))
	case n <= 30:
		return p.Fg(ui.Amber, fmt.Sprintf("expires in %d days", n))
	}
	return p.Muted(fmt.Sprintf("expires %s (in %d days)", e[:10], n))
}

// domainMenu is everything you can do with one of your domains.
func (s *shell) domainMenu(d string) *menu {
	p := s.p
	info := s.cachedDomain(d)
	title := p.BoldFg(ui.Green, d)
	lock, privacy := "Lock transfers", "Turn WHOIS privacy on"
	lockDesc, privacyDesc := "protects the domain from unwanted transfers", "hides the owner's details in WHOIS"
	lockCmd, privacyState := "lock", "on"
	lockDone, privacyDone := d+" is now locked against transfers", "WHOIS privacy is now on for "+d
	if info != nil {
		if st := domainStatusName(info["status"]); st != "" {
			title += "  " + p.Status(st)
		}
		if e := expiryText(p, info); e != "" {
			title += "  " + e
		}
		if on, _ := info["lockStatus"].(bool); on {
			lock, lockCmd, lockDone = "Unlock transfers", "unlock", d+" is unlocked: it can be transferred out now"
			lockDesc = "needed before a transfer to another registrar"
		}
		if on, _ := info["privacyProtectionStatus"].(bool); on {
			privacy, privacyState, privacyDone = "Turn WHOIS privacy off", "off", "WHOIS privacy is now off for "+d
			privacyDesc = "shows the owner's details in WHOIS again"
		}
	}
	run := func(words []string, label string, o runOpts) func(s *shell) tea.Cmd {
		return func(s *shell) tea.Cmd { return s.start(words, label, o) }
	}
	return &menu{
		title: title,
		items: []menuItem{
			{key: "i", label: "Details", desc: "dates, status, contacts, nameservers", act: run([]string{"domain", "info", d}, "Reading "+d, runOpts{})},
			{key: "d", label: "DNS records", desc: "see and add records", opens: true, act: func(s *shell) tea.Cmd { s.push(s.dnsMenu(d)); return nil }},
			{key: "r", label: "Renew…", desc: "choose the years, then confirm", opens: true, act: func(s *shell) tea.Cmd { s.push(s.renewForm(d)); return nil }},
			{key: "l", label: lock, desc: lockDesc, act: run([]string{"domain", lockCmd, d}, lock, runOpts{success: lockDone, onDone: toggleAfter(d, "lockStatus")})},
			{key: "p", label: privacy, desc: privacyDesc, act: run([]string{"domain", "privacy", d, privacyState}, privacy, runOpts{success: privacyDone, onDone: toggleAfter(d, "privacyProtectionStatus")})},
			{key: "n", label: "Nameservers…", desc: "point the domain to other DNS servers", opens: true, act: func(s *shell) tea.Cmd { s.push(s.nsForm(d)); return nil }},
			{key: "c", label: "Auth code", desc: "the code for a transfer to another registrar", act: run([]string{"domain", "info", d, "--jq", ".authCode"}, "Reading the auth code", runOpts{decorate: func(out string) string {
				return "  Auth code of " + d + ":  " + s.p.Bold(strings.TrimSpace(out))
			}})},
		},
		footer: "↑↓ choose · enter select · or press the letter · esc back",
	}
}

func (s *shell) dnsMenu(d string) *menu {
	return &menu{
		title: s.p.BoldFg(ui.Green, d) + s.p.Muted(" · DNS"),
		items: []menuItem{
			{key: "s", label: "Show the records", desc: "every record set of the zone", act: func(s *shell) tea.Cmd {
				return s.start([]string{"dns", "list", d}, "Reading the zone of "+d, runOpts{})
			}},
			{key: "a", label: "Add a record…", desc: "A, AAAA, CNAME, MX, TXT, NS, SRV, CAA", opens: true, act: func(s *shell) tea.Cmd { s.push(s.dnsForm(d)); return nil }},
			{key: "u", label: "Use the platform nameservers", desc: "needed for these records to resolve publicly", act: func(s *shell) tea.Cmd {
				return s.start([]string{"dns", "apply", d}, "Switching "+d+" to the platform nameservers", runOpts{success: d + " now uses the Domain Name API nameservers"})
			}},
		},
		footer: "↑↓ choose · enter select · or press the letter · esc back",
	}
}

// resultMenu offers what to do with a searched domain.
func (s *shell) resultMenu(info map[string]any) *menu {
	p := s.p
	d, _ := info["domainName"].(string)
	st, _ := info["status"].(string)
	price := priceOf(info)
	var title string
	var items []menuItem
	full := menuItem{key: "i", label: "Full result", desc: "the complete availability answer", act: func(s *shell) tea.Cmd {
		return s.start([]string{"domain", "search", d}, "Asking the registry", runOpts{})
	}}
	switch strings.ToUpper(st) {
	case "AVAILABLE":
		title = p.BoldFg(ui.Green, "✓ "+d) + p.Muted(" is available")
		if price != "" {
			title += p.Muted(" · ") + p.Bold(price) + p.Muted(" / year")
		}
		items = []menuItem{
			{key: "r", label: "Register…", desc: "years, nameservers and contact, then confirm", opens: true, act: func(s *shell) tea.Cmd { s.push(s.registerForm(d, price)); return nil }},
			full,
		}
	case "NOTAVAILABLE":
		title = p.Fg(ui.Red, "✗ "+d) + p.Muted(" is taken")
		items = []menuItem{
			{key: "t", label: "Transfer it in…", desc: "if it is yours at another registrar: check the auth code", opens: true, act: func(s *shell) tea.Cmd { s.push(s.transferForm(d)); return nil }},
			full,
		}
		if s.cachedDomain(d) != nil {
			items = append([]menuItem{{key: "m", label: "Manage it", desc: "it is in your account", opens: true, act: func(s *shell) tea.Cmd { s.push(s.domainMenu(d)); return nil }}}, items...)
		}
	default:
		title = p.Fg(ui.Amber, "– "+d) + p.Muted(" cannot be registered here")
		items = []menuItem{full, {key: "e", label: "Every extension and its price", act: func(s *shell) tea.Cmd {
			return s.start([]string{"product", "tlds", "--all"}, "Reading the price list", runOpts{})
		}}}
	}
	return &menu{title: title, items: items, footer: "↑↓ choose · enter select · or press the letter · esc back"}
}

func priceOf(info map[string]any) string {
	price, ok := info["price"].(float64)
	if !ok || price <= 0 {
		return ""
	}
	cur, _ := info["currency"].(string)
	return strings.TrimSpace(fmtMoney(price) + " " + cur)
}

// ---------------------------------------------------------------- forms

func yearsField(hint string) *formField {
	opts := make([]string, 10)
	for i := range opts {
		opts[i] = strconv.Itoa(i + 1)
	}
	return choiceField("years", "Years", opts, 0, hint, func(v string) string {
		if v == "1" {
			return "1 year"
		}
		return v + " years"
	})
}

func (s *shell) renewForm(d string) *form {
	return (&form{
		title:  s.p.Bold("Renew ") + s.p.BoldFg(ui.Green, d),
		fields: []*formField{yearsField("←→ to change; the new expiry date moves by this many years")},
		action: "Renew",
		submit: func(s *shell, v map[string]string) tea.Cmd {
			return s.runCommand([]string{"domain", "renew", d, "--period", v["years"]})
		},
	}).init()
}

func (s *shell) registerForm(d, price string) *form {
	ns := s.prefs.Nameservers
	if ns == "" {
		ns = platformNameservers
	}
	f := &form{
		title: s.p.Bold("Register ") + s.p.BoldFg(ui.Green, d),
		intro: "The contact handle is used for all four roles (registrant, admin, tech, billing).",
		fields: []*formField{
			yearsField("←→ to change"),
			textField("ns", "Nameservers", ns, platformNameservers, "comma separated; the platform nameservers let you manage DNS here", true),
			textField("handle", "Contact handle", s.prefs.Handle, "e.g. DNA-12345", "an existing contact handle; create one with `dna contact create`", true),
		},
		action: "Continue",
	}
	if price != "" {
		f.note = func(f *form) string {
			years, _ := strconv.Atoi(f.values()["years"])
			amount, cur, _ := strings.Cut(price, " ")
			x, err := strconv.ParseFloat(strings.ReplaceAll(amount, ",", ""), 64)
			if err != nil {
				return ""
			}
			return s.p.Muted("Price  ") + s.p.Bold(fmtMoney(x*float64(years))+" "+cur) + s.p.Muted(fmt.Sprintf("  (%s × %d)", price, years))
		}
	}
	f.submit = func(s *shell, v map[string]string) tea.Cmd {
		s.prefs.Handle, s.prefs.Nameservers = v["handle"], v["ns"]
		s.savePrefs()
		if f.note != nil {
			s.pendingNote = f.note(f)
		}
		return s.runCommand([]string{"domain", "register", d, "--period", v["years"], "--ns", strings.ReplaceAll(v["ns"], " ", ""), "--handle", v["handle"]})
	}
	return f.init()
}

var recordTypes = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA"}

var recordHints = map[string]string{
	"A": "an IPv4 address, e.g. 192.0.2.10", "AAAA": "an IPv6 address, e.g. 2001:db8::10",
	"CNAME": "a host name, e.g. target.example.net.", "MX": "priority and mail server, e.g. 10 mail.example.com.",
	"TXT": "any text, e.g. v=spf1 include:_spf.example.net ~all (quotes are added for you)", "NS": "a nameserver, e.g. ns1.example.net.",
	"SRV": "priority weight port target, e.g. 10 5 5060 sip.example.com.", "CAA": `flag tag value, e.g. 0 issue "letsencrypt.org"`,
}

func (s *shell) dnsForm(d string) *form {
	f := &form{
		title: s.p.Bold("Add a DNS record to ") + s.p.BoldFg(ui.Green, d),
		fields: []*formField{
			choiceField("type", "Type", recordTypes, 0, "←→ to change the record type", nil),
			textField("name", "Name", "@", "@", "@ is the domain itself; www makes www."+d, true),
			textField("value", "Value", "", "", "", true),
			choiceField("ttl", "TTL", []string{"300", "3600", "86400"}, 1, "how long resolvers may cache it", func(v string) string {
				return map[string]string{"300": "5 minutes", "3600": "1 hour", "86400": "1 day"}[v]
			}),
		},
		action: "Add the record",
	}
	f.note = func(f *form) string { return s.p.Muted("Value: " + recordHints[f.values()["type"]]) }
	f.submit = func(s *shell, v map[string]string) tea.Cmd {
		words := []string{"dns", "add", d, "--type", v["type"], "--name", v["name"], "--content", v["value"], "--ttl", v["ttl"]}
		return s.start(words, "Adding the record", runOpts{success: fmt.Sprintf("%s record %s added to %s", v["type"], v["name"], d), onDone: func(s *shell, code int) tea.Cmd {
			if code != 0 {
				return nil
			}
			return s.start([]string{"dns", "list", d}, "Reading the zone of "+d, runOpts{})
		}})
	}
	return f.init()
}

func (s *shell) nsForm(d string) *form {
	var current []string
	if info := s.cachedDomain(d); info != nil {
		if list, ok := info["nameservers"].([]any); ok {
			for _, x := range list {
				switch v := x.(type) {
				case map[string]any:
					if n, _ := v["name"].(string); n != "" {
						current = append(current, n)
					}
				case string:
					current = append(current, v)
				}
			}
		}
	}
	val := strings.Join(current, ", ")
	f := &form{
		title:  s.p.Bold("Nameservers of ") + s.p.BoldFg(ui.Green, d),
		intro:  "Comma separated. Use " + platformNameservers + " to manage DNS records here.",
		fields: []*formField{textField("ns", "Nameservers", val, platformNameservers, "at least two, e.g. ns1.example.net, ns2.example.net", true)},
		action: "Save",
	}
	f.submit = func(s *shell, v map[string]string) tea.Cmd {
		words := []string{"domain", "nameservers", "set", d}
		for _, n := range strings.Split(v["ns"], ",") {
			if n = strings.TrimSpace(n); n != "" {
				words = append(words, n)
			}
		}
		return s.start(words, "Updating the nameservers", runOpts{success: "Nameservers of " + d + " updated", onDone: invalidateAfter})
	}
	return f.init()
}

func (s *shell) transferForm(d string) *form {
	return (&form{
		title:  s.p.Bold("Transfer ") + s.p.BoldFg(ui.Green, d) + s.p.Bold(" in"),
		intro:  "First we check the auth code with the registry; nothing is charged yet.",
		fields: []*formField{textField("code", "Auth code", "", "from the current registrar", "also called EPP code or transfer key", true)},
		action: "Check",
		submit: func(s *shell, v map[string]string) tea.Cmd {
			return s.start([]string{"domain", "transfer-check", d, v["code"]}, "Checking the transfer", runOpts{})
		},
	}).init()
}

// registerStart asks for a domain, checks it and continues with the
// registration form when it is free.
func (s *shell) registerStart() tea.Cmd {
	s.push((&form{
		title:  s.p.Bold("Register a domain"),
		fields: []*formField{textField("domain", "Domain", "", "e.g. acme-store.com", "the full name with its extension", true)},
		action: "Check availability",
		submit: func(s *shell, v map[string]string) tea.Cmd {
			d := strings.ToLower(strings.TrimSpace(v["domain"]))
			return s.checkThen(d, func(s *shell, info map[string]any) tea.Cmd {
				if st, _ := info["status"].(string); strings.EqualFold(st, "AVAILABLE") {
					s.push(s.registerForm(d, priceOf(info)))
					return nil
				}
				s.push(s.resultMenu(info))
				return nil
			})
		},
	}).init())
	return nil
}

// ---------------------------------------------------------------- availability check

type availMsg struct {
	info map[string]any
	err  string
	then func(s *shell, info map[string]any) tea.Cmd
}

func (s *shell) checkThen(d string, then func(s *shell, info map[string]any) tea.Cmd) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	s.run = runState{active: true, label: "Checking " + d, start: time.Now(), cancel: cancel}
	app, args := s.app, s.args([]string{"domain", "search", d, "-o", "json"})
	return tea.Batch(s.tick(), func() tea.Msg {
		defer cancel()
		var out, errb bytes.Buffer
		c := app.child(&out, &errb, 0)
		c.StdoutTTY = false
		if code := c.Run(ctx, args); code != 0 {
			return availMsg{err: strings.TrimRight(errb.String(), "\n")}
		}
		var res struct {
			Info map[string]any `json:"info"`
		}
		if err := json.Unmarshal(out.Bytes(), &res); err != nil || res.Info == nil {
			return availMsg{err: "  unexpected answer from the API"}
		}
		return availMsg{info: res.Info, then: then}
	})
}

func (s *shell) onAvail(m availMsg) tea.Cmd {
	s.run = runState{}
	if m.err != "" {
		return tea.Println(m.err)
	}
	return m.then(s, m.info)
}

// ---------------------------------------------------------------- domain picker

// pickDomain opens a searchable list of the account's domains. Typing a
// full domain name that is not in the list is allowed too.
func (s *shell) pickDomain(title string, then func(s *shell, d string) tea.Cmd, opens bool) tea.Cmd {
	m := &menu{
		title:      s.p.Bold(title),
		filterable: true,
		empty:      "No domain matches. Type the full name (with its extension) to use it anyway.",
		loading:    func(s *shell) bool { return s.domains.state == domainsLoading },
	}
	m.build = func(s *shell, filter string) []menuItem {
		var items []menuItem
		exact := false
		for _, it := range s.domains.items {
			d, _ := it["domainName"].(string)
			if strings.EqualFold(d, filter) {
				exact = true
			}
			dd := d
			items = append(items, menuItem{label: d, desc: ui.Strip(expiryText(s.p, it)), opens: opens, act: func(s *shell) tea.Cmd { return then(s, dd) }})
		}
		f := strings.ToLower(strings.TrimSpace(filter))
		if !exact && strings.Contains(f, ".") && domainish.MatchString(f) {
			items = append(items, menuItem{label: "Use " + f, desc: "not in the list, use it anyway", keep: true, opens: opens, act: func(s *shell) tea.Cmd { return then(s, f) }})
		}
		return items
	}
	s.push(m)
	return s.ensureDomains()
}
