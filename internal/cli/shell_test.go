package cli

import (
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testShell(t *testing.T, api *fakeAPI, width int) *shell {
	t.Helper()
	env := map[string]string{
		"DNA_CONFIG":      filepath.Join(t.TempDir(), "config.json"),
		"DNA_RESELLER_ID": testReseller,
		"DNA_API_KEY":     "k",
		"DNA_ENV":         "ote",
		"COLORTERM":       "truecolor",
	}
	if api != nil {
		env["DNA_BASE_URL"] = api.srv.URL
	}
	// the same programs on every machine: dna at a short path, no agents
	old := lookPath
	lookPath = func(name string) (string, error) {
		if name == "dna" {
			return "/usr/local/bin/dna", nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPath = old })
	a := &App{Getenv: func(k string) string { return env[k] }, Build: BuildInfo{Version: "1.2.0"},
		StdinTTY: true, StdoutTTY: true, StderrTTY: true, Width: width}
	s := newShell(a)
	s.status = statusInfo{loaded: true, env: "ote"}
	return s
}

// drain runs a command and the batches it returns, collecting messages.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, drain(c)...)
		}
		return out
	}
	// tea.Sequence returns an unexported []tea.Cmd; run its commands too
	if v := reflect.ValueOf(msg); v.IsValid() && v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf((tea.Cmd)(nil)) {
		var out []tea.Msg
		for i := 0; i < v.Len(); i++ {
			if c, ok := v.Index(i).Interface().(tea.Cmd); ok {
				out = append(out, drain(c)...)
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

func init() { cursorBlink = false } // tests must not wait for blink timers

// feed delivers the messages of cmd (except animation ticks) to the shell.
func feed(s *shell, cmd tea.Cmd) {
	for _, m := range drain(cmd) {
		switch m.(type) {
		case frameMsg, nil:
			continue
		}
		_, next := s.Update(m)
		feed(s, next)
	}
}

func keys(s *shell, ks ...string) {
	for _, k := range ks {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := s.Update(msg)
		feed(s, cmd)
	}
}

func screen(s *shell) string { return ansiStrip(s.View()) }

const listJSON = `{"totalCount":2,"items":[
	{"domainName":"acme-shop.com","status":0,"expirationDate":"2099-01-01T00:00:00","lockStatus":false,"privacyProtectionStatus":true,"nameservers":[{"name":"tr.apiname.com"},{"name":"eu.apiname.com"}]},
	{"domainName":"brandname.net","status":0,"expirationDate":"2099-02-01T00:00:00","lockStatus":true,"privacyProtectionStatus":false}]}`

func TestShellWelcomeAndHome(t *testing.T) {
	s := testShell(t, nil, 120)
	welcome := ansiStrip(s.welcome())
	for _, want := range []string{"██████╗", "Domain Name API", "v1.2.0", "quick actions"} {
		if !strings.Contains(welcome, want) {
			t.Errorf("welcome missing %q:\n%s", want, welcome)
		}
	}
	s.status.acct = &account{ResellerName: "Acme", UsdBalance: 1284.5, TryBalance: 10}
	view := screen(s)
	for _, want := range []string{"1  My domains", "2  Register a domain", "DNS records", "Switch to production", "OTE", "1,284.50 USD", "↓ choose an action"} {
		if !strings.Contains(view, want) {
			t.Errorf("home view missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(s.View(), "\n") {
		if w := len([]rune(ansiStrip(line))); w > 120 {
			t.Errorf("line wider than the terminal (%d): %s", w, ansiStrip(line))
		}
	}
	if w := ansiStrip(testShell(t, nil, 70).welcome()); strings.Contains(w, "██████╗") || !strings.Contains(w, "╔╦╗") {
		t.Errorf("narrow welcome:\n%s", w)
	}
}

// The main flow needs no typing at all: ↓, Enter, Enter, a letter.
func TestShellBrowseAndLockWithoutTyping(t *testing.T) {
	api := newFakeAPI(t)
	api.json("GET /api/v1/domains", listJSON)
	api.json("POST /api/v1/domains/lock", ``)
	s := testShell(t, api, 120)

	keys(s, "down")
	if s.focus != focusHome {
		t.Fatal("↓ on an empty prompt should focus the quick actions")
	}
	keys(s, "enter") // My domains
	if _, ok := s.top().(*browser); !ok {
		t.Fatalf("enter on My domains should open the browser, top = %T", s.top())
	}
	if v := screen(s); !strings.Contains(v, "acme-shop.com") || !strings.Contains(v, "brandname.net") {
		t.Fatalf("browser view:\n%s", v)
	}
	keys(s, "enter") // acme-shop.com → its menu
	m, ok := s.top().(*menu)
	if !ok || !strings.Contains(ansiStrip(m.title), "acme-shop.com") {
		t.Fatalf("enter on a domain should open its menu, top = %T", s.top())
	}
	if v := screen(s); !strings.Contains(v, "Lock transfers") || !strings.Contains(v, "Turn WHOIS privacy off") || !strings.Contains(v, "Renew…") {
		t.Errorf("domain menu should reflect the domain's state:\n%s", v)
	}
	keys(s, "l")
	if last := api.last(); last.Method != "POST" || last.Path != "/api/v1/domains/lock" {
		t.Fatalf("lock request = %+v", last)
	}
	if _, ok := s.top().(*browser); !ok {
		t.Errorf("after the action the browser should be back, top = %T", s.top())
	}
	if on, _ := s.cachedDomain("acme-shop.com")["lockStatus"].(bool); !on {
		t.Error("lock state not updated in the list")
	}
	keys(s, "esc")
	if s.top() != nil {
		t.Error("esc should close the browser")
	}
}

func TestShellBrowserSearchTypesLetters(t *testing.T) {
	api := newFakeAPI(t)
	api.json("GET /api/v1/domains", listJSON)
	s := testShell(t, api, 100)
	feed(s, s.openBrowser())
	keys(s, "b", "r", "a")
	v := screen(s)
	if !strings.Contains(v, "brandname.net") || strings.Contains(v, "acme-shop.com") {
		t.Errorf("typing should search, not trigger actions:\n%s", v)
	}
	if n := len(api.all()); n != 1 {
		t.Errorf("only the list should be requested, got %d requests", n)
	}
}

func TestShellCompletion(t *testing.T) {
	s := testShell(t, nil, 100)
	s.setInput("/do")
	if got := labels(s.sugg); !strings.Contains(got, "/domains") || !strings.Contains(got, "/doctor") || strings.Contains(got, "/balance") {
		t.Errorf("slash suggestions = %s", got)
	}
	s.setInput("domain re")
	if got := labels(s.sugg); !strings.Contains(got, "domain register") || !strings.Contains(got, "domain renew") {
		t.Errorf("command suggestions = %s", got)
	}
	s.setInput("acme-store")
	if len(s.sugg) != 0 {
		t.Errorf("a domain name should not open the command menu: %s", labels(s.sugg))
	}
	// The account's domains complete domain arguments.
	s.domains = domainCache{state: domainsLoaded, items: []map[string]any{{"domainName": "acme-shop.com"}, {"domainName": "brandname.net"}}}
	s.setInput("domain info ac")
	if got := labels(s.sugg); got != "acme-shop.com" {
		t.Errorf("domain argument suggestions = %q", got)
	}
	s.setInput("/dns br")
	if len(s.sugg) != 1 || s.sugg[0].value != "/dns brandname.net" || !s.sugg[0].run {
		t.Errorf("slash domain suggestions = %+v", s.sugg)
	}
}

func labels(sugg []suggestion) string {
	var l []string
	for _, s := range sugg {
		l = append(l, s.label)
	}
	return strings.Join(l, " | ")
}

func TestShellSlashWithoutDomainOpensPicker(t *testing.T) {
	api := newFakeAPI(t)
	api.json("GET /api/v1/domains", listJSON)
	s := testShell(t, api, 100)
	feed(s, s.slash("/dns"))
	m, ok := s.top().(*menu)
	if !ok || !m.filterable {
		t.Fatalf("/dns without a domain should open the domain picker, top = %T", s.top())
	}
	keys(s, "b", "r")
	if v := screen(s); !strings.Contains(v, "brandname.net") || strings.Contains(v, "acme-shop.com") {
		t.Errorf("picker search:\n%s", v)
	}
	// A domain that is not in the account can be typed in full.
	keys(s, "esc")
	for _, r := range "other.org" {
		keys(s, string(r))
	}
	if v := screen(s); !strings.Contains(v, "Use other.org") {
		t.Errorf("picker should offer a typed domain:\n%s", v)
	}
}

func TestShellLiveSearchAndPick(t *testing.T) {
	api := newFakeAPI(t)
	api.json("POST /api/v1/domains/bulk-search", `{"success":true,"infos":[{"domainName":"acme.com","status":"NOTAVAILABLE"},{"domainName":"acme.net","status":"AVAILABLE","price":11.49,"currency":"USD"}]}`)
	s := testShell(t, api, 100)
	s.in.SetValue("acme")
	s.onInputChange()
	feed(s, s.onLiveDebounce(liveDebounceMsg{seq: s.live.seq, query: "acme"}))
	view := screen(s)
	for _, want := range []string{"1 free of 2", "✗ acme.com", "✓ acme.net", "11.49 USD / year", "taken", "pick a result"} {
		if !strings.Contains(view, want) {
			t.Errorf("live view missing %q:\n%s", want, view)
		}
	}
	keys(s, "down", "down", "enter") // acme.net
	m, ok := s.top().(*menu)
	if !ok || !strings.Contains(ansiStrip(m.title), "acme.net is available") {
		t.Fatalf("enter on a free result should offer to register it, top = %T", s.top())
	}
	keys(s, "r")
	f, ok := s.top().(*form)
	if !ok || !strings.Contains(ansiStrip(f.title), "Register acme.net") {
		t.Fatalf("r should open the registration form, top = %T", s.top())
	}
	keys(s, "right") // 2 years
	if v := screen(s); !strings.Contains(v, "22.98 USD") || !strings.Contains(v, "tr.apiname.com") {
		t.Errorf("registration form should show the total and default nameservers:\n%s", v)
	}
	keys(s, "esc", "esc")
	if s.top() != nil {
		t.Errorf("esc should walk back to the prompt, top = %T", s.top())
	}
}

func TestShellLiveQuery(t *testing.T) {
	s := testShell(t, nil, 100)
	for in, want := range map[string]string{
		"acme": "acme", "Acme-Store.COM": "acme-store.com", "dom": "", "domain list": "", "/dns": "", "a": "", "acme.com.tr": "acme.com.tr",
	} {
		s.in.SetValue(in)
		if got := s.liveQuery(); got != want {
			t.Errorf("liveQuery(%q) = %q, want %q", in, got, want)
		}
	}
	if got := liveNames("acme.io"); got[0] != "acme.io" || strings.Contains(strings.Join(got[1:], " "), "acme.io") || len(got) > 8 {
		t.Errorf("liveNames = %v", got)
	}
	if got := strings.Join(searchArgs("Acme"), " "); got != "domain search acme.com" {
		t.Errorf("searchArgs(acme) = %v, want the .com alone", got)
	}
	if got := strings.Join(searchArgs("acme.io"), " "); got != "domain search acme.io" {
		t.Errorf("searchArgs(acme.io) = %v", got)
	}
	if got := compareArgs("acme"); got[1] != "bulk-search" || got[2] != "acme.com" || len(got) != 2+len(popularTLDs) {
		t.Errorf("compareArgs = %v", got)
	}
}

func TestShellRenewFormNeedsConfirmation(t *testing.T) {
	api := newFakeAPI(t)
	api.json("POST /api/v1/domains/renew", `{"success":true}`)
	s := testShell(t, api, 100)
	s.push(s.renewForm("example.com"))
	keys(s, "right", "enter") // 2 years, enter on the last field submits
	c, ok := s.top().(*confirmBox)
	if !ok {
		t.Fatalf("submitting a renewal should ask first, top = %T", s.top())
	}
	if n := len(api.all()); n != 0 {
		t.Fatalf("%d requests sent before confirmation", n)
	}
	box := ansiStrip(strings.Join(c.view(s, 100), "\n"))
	for _, want := range []string{"BILLABLE", "Renew a domain", "example.com", "period", "2", "OTE", "Cancel", "Renew"} {
		if !strings.Contains(box, want) {
			t.Errorf("confirmation missing %q:\n%s", want, box)
		}
	}
	keys(s, "enter") // Cancel is the default
	if s.top() != nil || len(api.all()) != 0 {
		t.Fatal("the default choice must cancel without sending")
	}
	feed(s, s.runCommand([]string{"domain", "renew", "example.com", "--period", "1"}))
	keys(s, "y")
	renewals := 0
	for _, r := range api.all() {
		if r.Path == "/api/v1/domains/renew" {
			renewals++
		}
	}
	if renewals != 1 {
		t.Fatalf("confirmed renew should send exactly one renewal: %v", api.all())
	}
	if last := api.last(); last.Path != "/api/v1/deposit/accounts/me" {
		t.Errorf("the balance should be refreshed after a billable step, last = %+v", last)
	}
}

func TestShellDNSForm(t *testing.T) {
	api := newFakeAPI(t)
	api.json("POST /api/v1/domains/zones", ``)
	api.json("GET /api/v1/domains/zones", `[]`)
	s := testShell(t, api, 100)
	s.push(s.dnsForm("example.com"))
	keys(s, "right", "right") // CNAME
	keys(s, "down")           // name
	keys(s, "down")           // value
	for _, r := range "target.example.net." {
		keys(s, string(r))
	}
	keys(s, "down", "down", "enter")
	var add *recorded
	for _, r := range api.all() {
		if r.Method == "POST" && r.Path == "/api/v1/domains/zones" {
			rr := r
			add = &rr
		}
	}
	if add == nil || !strings.Contains(add.Body, `"CNAME"`) || !strings.Contains(add.Body, "target.example.net.") {
		t.Fatalf("dns add request = %+v", add)
	}
	if last := api.last(); last.Method != "GET" || last.Path != "/api/v1/domains/zones" {
		t.Errorf("the zone should be shown after adding, last = %+v", last)
	}
	// A required field left empty keeps the form open with an error.
	s.push(s.dnsForm("example.com"))
	keys(s, "down", "down", "down", "down", "enter")
	if f, ok := s.top().(*form); !ok || !strings.Contains(f.err, "Value") {
		t.Errorf("empty value should be refused, top = %T", s.top())
	}
}

// On a narrow terminal no line may be wider than the screen: a wrapped line
// breaks the redraw of the whole shell.
func TestShellNarrowNeverWraps(t *testing.T) {
	api := newFakeAPI(t)
	api.json("GET /api/v1/domains", listJSON)
	s := testShell(t, api, 46)
	check := func(where string) {
		t.Helper()
		for _, l := range strings.Split(s.View(), "\n") {
			if w := len([]rune(ansiStrip(l))); w > 45 {
				t.Errorf("%s: line of %d cells: %s", where, w, ansiStrip(l))
			}
		}
	}
	check("home")
	for _, line := range strings.Split(ansiStrip(s.welcome()), "\n") {
		if len([]rune(line)) > 45 {
			t.Errorf("welcome line too wide: %s", line)
		}
	}
	s.push(s.registerForm("brandname.net", "11.49 USD", s.contacts()))
	check("register form")
	s.pop()
	feed(s, s.agentPicker())
	keys(s, "enter")
	check("agent setup")
	if got := confirmValue([]any{map[string]any{"contactType": "Registrant", "handle": "DNA-1"}, map[string]any{"contactType": "Admin", "handle": "DNA-1"},
		map[string]any{"contactType": "Tech", "handle": "DNA-1"}, map[string]any{"contactType": "Billing", "handle": "DNA-1"}}); got != "DNA-1 (all roles)" {
		t.Errorf("confirmValue(handles) = %q", got)
	}
	if got := confirmValue([]any{"ns1.example.net", "ns2.example.net"}); got != "ns1.example.net, ns2.example.net" {
		t.Errorf("confirmValue(list) = %q", got)
	}
}

func TestShellRunsCommands(t *testing.T) {
	api := newFakeAPI(t)
	api.json("GET /api/v1/deposit/accounts/me", `{"resellerName":"Acme","resellerGroupName":"Gold","usdBalance":5,"tryBalance":7}`)
	s := testShell(t, api, 100)
	var done *runDoneMsg
	for _, m := range drain(s.start([]string{"account", "get"}, "x", runOpts{})) {
		if d, ok := m.(runDoneMsg); ok {
			done = &d
		}
	}
	if done == nil || done.code != 0 || !strings.Contains(ansiStrip(done.out), "Acme") || !strings.Contains(done.out, "\x1b[") {
		t.Fatalf("shell command output: %+v", done)
	}
	if words, err := splitWords(`dns add example.com --type TXT --content "v=spf1 ~all"`); err != nil || words[len(words)-1] != "v=spf1 ~all" {
		t.Errorf("splitWords = %q, %v", words, err)
	}
	if _, err := splitWords(`say "unterminated`); err == nil {
		t.Error("unterminated quote accepted")
	}
}

func TestMenuLabelsFitNarrow(t *testing.T) {
	s := newShell(&App{Getenv: func(string) string { return "" }})
	m := &menu{items: []menuItem{{label: "Short", desc: "first description"}, {label: "A very long label that would push everything", desc: "second description"}}}
	for _, l := range m.view(s, 50) {
		if strings.Contains(l, "Short") && !strings.Contains(l, "first descr") {
			t.Errorf("a long label pushed the description off: %q", l)
		}
	}
}

// A name typed without an extension and Enter checks its .com alone: one
// quick search, not all popular extensions (slow registries hold those).
func TestShellEnterOnNameChecksCom(t *testing.T) {
	api := newFakeAPI(t)
	api.json("POST /api/v1/domains/search", `{"success":true,"info":{"domainName":"zqxw.com","status":"AVAILABLE","price":11.99,"currency":"USD","period":1}}`)
	s := testShell(t, api, 100)
	feed(s, s.submit("zqxw"))
	var paths []string
	for _, r := range api.all() {
		paths = append(paths, r.Method+" "+r.Path+" "+r.Body)
	}
	got := strings.Join(paths, "\n")
	if !strings.Contains(got, `POST /api/v1/domains/search {"domainName":"zqxw.com"}`) || strings.Contains(got, "bulk-search") {
		t.Errorf("enter on a bare name should search the .com only, requests:\n%s", got)
	}
}

// The live list shows the first name as soon as it answers, then the rest.
func TestShellLiveQuickFirstName(t *testing.T) {
	s := testShell(t, nil, 100)
	s.in.SetValue("acme")
	s.onInputChange()
	seq := s.live.seq
	s.live.busy = true
	com := []map[string]any{{"domainName": "acme.com", "status": "AVAILABLE", "price": 11.99, "currency": "USD"}}
	all := append(com, map[string]any{"domainName": "acme.net", "status": "NOTAVAILABLE"})

	s.onLiveResult(liveResultMsg{seq: seq, query: "acme", results: com, partial: true})
	if v := screen(s); !strings.Contains(v, "✓ acme.com") || !strings.Contains(v, "checking the other extensions") {
		t.Errorf("the quick .com answer should show while the rest load:\n%s", v)
	}
	s.onLiveResult(liveResultMsg{seq: seq, query: "acme", results: all})
	s.onLiveResult(liveResultMsg{seq: seq, query: "acme", results: com, partial: true}) // late: ignored
	if len(s.live.results) != 2 || s.live.busy {
		t.Errorf("the full list should replace the quick answer and stay, got %d results busy=%v", len(s.live.results), s.live.busy)
	}
	if v := screen(s); !strings.Contains(v, "1 free of 2") || !strings.Contains(v, "enter checks acme.com") {
		t.Errorf("live view after the full list:\n%s", v)
	}

	// the bulk search fails after the quick answer: keep the .com
	s.live.busy = true
	s.onLiveResult(liveResultMsg{seq: seq, query: "acme", results: com, partial: true})
	s.onLiveResult(liveResultMsg{seq: seq, query: "acme", err: context.DeadlineExceeded})
	if v := screen(s); !strings.Contains(v, "✓ acme.com") || !strings.Contains(v, "the other extensions did not answer") {
		t.Errorf("a failed bulk search should keep the .com answer:\n%s", v)
	}
}
