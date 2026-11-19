package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/domainreseller/cli/internal/config"
)

func TestSplitPhone(t *testing.T) {
	for _, tc := range []struct{ raw, code, country, wantCode, wantNum string }{
		{"+90 555 123 45 67", "", "", "90", "5551234567"},
		{"0090 (555) 123-4567", "", "", "90", "5551234567"},
		{"0555 123 45 67", "", "tr", "90", "5551234567"},
		{"5551234567", "90", "", "90", "5551234567"},
		{"+1 212 555 0100", "", "", "1", "2125550100"},
		{"+44 20 7946 0958", "", "", "44", "2079460958"},
		{"06 1234 5678", "", "IT", "39", "0612345678"}, // Italy keeps the leading 0
	} {
		code, num, err := splitPhone(tc.raw, tc.code, tc.country)
		if err != nil || code != tc.wantCode || num != tc.wantNum {
			t.Errorf("splitPhone(%q, %q, %q) = %q, %q, %v; want %q, %q", tc.raw, tc.code, tc.country, code, num, err, tc.wantCode, tc.wantNum)
		}
	}
	if _, _, err := splitPhone("555 123 45 67", "", ""); err == nil || !strings.Contains(err.Error(), "+90") {
		t.Errorf("a number without any country should ask for the code, got %v", err)
	}
	m := map[string]any{"phone": "+90 555 123 45 67", "country": "tr"}
	if err := normalizeContact(m); err != nil || m["phoneCountryCode"] != "90" || m["phone"] != "5551234567" || m["country"] != "TR" {
		t.Errorf("normalizeContact = %v, %v", m, err)
	}
	m = map[string]any{"phone": "+90 555 123 45 67"}
	if _ = normalizeContact(m); m["country"] != "TR" {
		t.Errorf("country should come from +90: %v", m)
	}
	m = map[string]any{"phone": "+1 212 555 0100"}
	if _ = normalizeContact(m); m["country"] != nil {
		t.Errorf("+1 is shared by many countries, country must stay empty: %v", m)
	}
}

// contactAPI answers the calls of the contact flows.
func contactAPI(t *testing.T) *fakeAPI {
	api := newFakeAPI(t)
	api.json("POST /api/v1/contacts", `{"handle":"D-NEW1","isSuccess":true}`)
	api.json("POST /api/v1/domains/register", `{"success":true,"status":"Active"}`)
	api.json("GET /api/v1/contacts/D-77", `{"contact":{"handle":"D-77","firstName":"Grace","lastName":"Hopper","eMail":"grace@example.com","country":"US"}}`)
	api.json("GET /api/v1/domains", `{"totalCount":2,"items":[{"domainName":"a.com"},{"domainName":"b.com"}]}`)
	contacts := func(h string) string {
		var parts []string
		for _, r := range []string{"Registrant", "Admin", "Tech", "Billing"} {
			parts = append(parts, `{"contactType":"`+r+`","handle":"`+h+`","firstName":"Ada","lastName":"Lovelace","eMail":"ada@example.com","country":"TR"}`)
		}
		return `{"domainName":"x","contacts":[` + strings.Join(parts, ",") + `]}`
	}
	api.routes["GET /api/v1/domains/info"] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, contacts("D-1"))
	}
	return api
}

func registerBody(t *testing.T, api *fakeAPI) map[string]any {
	t.Helper()
	for _, r := range api.all() {
		if r.Path == "/api/v1/domains/register" {
			return decode(t, r.Body)
		}
	}
	t.Fatal("no registration was sent")
	return nil
}

func handlesOf(body map[string]any) []string {
	var out []string
	hs, _ := body["handles"].([]any)
	for _, h := range hs {
		m, _ := h.(map[string]any)
		out = append(out, m["contactType"].(string)+"="+m["handle"].(string))
	}
	return out
}

func TestContactCreateBecomesTheDefault(t *testing.T) {
	api := contactAPI(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	env := map[string]string{"DNA_CONFIG": cfg}

	r := run(t, api, env, "", "contact", "create", "--first-name", "Ada", "--last-name", "Lovelace", "--email", "ada@example.com",
		"--phone", "+90 555 123 45 67", "--address", "1 Example St", "--city", "Istanbul", "--state", "Istanbul", "--postal-code", "34000")
	if r.code != 0 {
		t.Fatalf("contact create: %d %s", r.code, r.stderr)
	}
	body := decode(t, api.last().Body)
	c, _ := body["contact"].(map[string]any)
	if c["phoneCountryCode"] != "90" || c["phone"] != "5551234567" || c["country"] != "TR" {
		t.Errorf("phone not split / country not derived: %v", c)
	}

	r = run(t, api, env, "", "contact", "list", "-o", "json")
	list := decode(t, r.stdout)
	if list["default"] != "D-NEW1" {
		t.Fatalf("new contact is not the default: %s", r.stdout)
	}
	if !strings.Contains(r.stdout, `"name": "Ada Lovelace"`) && !strings.Contains(r.stdout, `"name":"Ada Lovelace"`) {
		t.Errorf("contact details not saved: %s", r.stdout)
	}

	// registration without --handle and --ns: the default and the platform nameservers
	r = run(t, api, env, "", "domain", "register", "example.com", "--period", "1", "--yes")
	if r.code != 0 {
		t.Fatalf("register: %d %s", r.code, r.stderr)
	}
	reg := registerBody(t, api)
	if got := strings.Join(handlesOf(reg), ","); got != "Registrant=D-NEW1,Admin=D-NEW1,Tech=D-NEW1,Billing=D-NEW1" {
		t.Errorf("handles = %s", got)
	}
	if ns, _ := json.Marshal(reg["nameServers"]); string(ns) != `["tr.apiname.com","eu.apiname.com"]` {
		t.Errorf("nameservers = %s", ns)
	}
}

func TestRegisterWithoutContactExplainsWhatToDo(t *testing.T) {
	api := contactAPI(t)
	r := run(t, api, nil, "", "domain", "register", "example.com", "--yes")
	if r.code != ExitUsage {
		t.Fatalf("exit %d, want %d: %s", r.code, ExitUsage, r.stderr)
	}
	for _, want := range []string{"dna contact default", "dna contact create", "--scan"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("error does not mention %q: %s", want, r.stderr)
		}
	}
	if len(api.all()) != 0 {
		t.Errorf("nothing should be sent: %v", api.all())
	}
}

// interactiveRun runs dna as a person at a terminal answering stdin.
func interactiveRun(t *testing.T, api *fakeAPI, cfg, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	env := map[string]string{"DNA_CONFIG": cfg, "DNA_RESELLER_ID": testReseller, "DNA_API_KEY": "k", "DNA_BASE_URL": api.srv.URL}
	app := &App{In: strings.NewReader(stdin), Out: &out, Err: &errb, Getenv: func(k string) string { return env[k] },
		Build: BuildInfo{Version: "test"}, StdinTTY: true, StderrTTY: true}
	code := app.Run(context.Background(), args)
	return result{code, out.String(), errb.String()}
}

func TestContactCreateAsksForMissingDetails(t *testing.T) {
	api := contactAPI(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	// first, last, e-mail (one wrong try), phone, country (default TR), city, state (default city), address, postal code
	answers := "Ada\nLovelace\nnot-an-email\nada@example.com\n+90 555 123 45 67\n\nIstanbul\n\n1 Example St\n34000\n"
	r := interactiveRun(t, api, cfg, answers, "contact", "create")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	c, _ := decode(t, api.last().Body)["contact"].(map[string]any)
	want := map[string]string{"firstName": "Ada", "eMail": "ada@example.com", "phoneCountryCode": "90", "phone": "5551234567",
		"country": "TR", "city": "Istanbul", "state": "Istanbul", "postalCode": "34000"}
	for k, v := range want {
		if c[k] != v {
			t.Errorf("%s = %v, want %s", k, c[k], v)
		}
	}
	if !strings.Contains(r.stderr, "does not look like an e-mail") {
		t.Errorf("the wrong e-mail was not explained: %s", r.stderr)
	}
}

func TestRegisterCreatesAContactOnTheSpot(t *testing.T) {
	api := contactAPI(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	answers := "1\nAda\nLovelace\nada@example.com\n+90 555 123 45 67\n\nIstanbul\n\n1 Example St\n34000\ny\n"
	r := interactiveRun(t, api, cfg, answers, "domain", "register", "example.com")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if got := strings.Join(handlesOf(registerBody(t, api)), ","); !strings.Contains(got, "Registrant=D-NEW1") {
		t.Errorf("registration did not use the new contact: %s", got)
	}
	f, _ := config.Load(cfg)
	if _, def := f.Contacts("production"); def != "D-NEW1" {
		t.Errorf("new contact not the default: %q", def)
	}
}

func TestContactScanDefaultAndForget(t *testing.T) {
	api := contactAPI(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	env := map[string]string{"DNA_CONFIG": cfg}

	r := run(t, api, env, "", "contact", "list", "--scan", "-o", "json")
	list := decode(t, r.stdout)
	if list["default"] != "D-1" {
		t.Fatalf("one handle on every domain should become the default: %s %s", r.stdout, r.stderr)
	}

	r = run(t, api, env, "", "contact", "default", "D-77", "-o", "json")
	if def := decode(t, r.stdout); def["default"] != "D-77" {
		t.Fatalf("contact default: %s %s", r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "Grace Hopper") {
		t.Errorf("details of D-77 not saved: %s", r.stdout)
	}

	r = run(t, api, env, "", "contact", "forget", "D-77")
	if r.code != 0 {
		t.Fatalf("forget: %d %s", r.code, r.stderr)
	}
	r = run(t, api, env, "", "contact", "list", "-o", "json")
	if list := decode(t, r.stdout); list["default"] != "" || strings.Contains(r.stdout, "D-77") {
		t.Errorf("forgotten contact still listed: %s", r.stdout)
	}
	if r := run(t, api, env, "", "contact", "forget", "D-404"); r.code != ExitUsage {
		t.Errorf("forgetting an unknown handle: exit %d", r.code)
	}
}

func TestDomainInfoRemembersContacts(t *testing.T) {
	api := contactAPI(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	if r := run(t, api, map[string]string{"DNA_CONFIG": cfg}, "", "domain", "info", "a.com", "-o", "json"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	f, _ := config.Load(cfg)
	cs, def := f.Contacts("production")
	if len(cs) != 1 || cs[0].Handle != "D-1" || cs[0].Name != "Ada Lovelace" || def != "" {
		t.Errorf("contacts = %+v, default %q (seen contacts never become the default on their own)", cs, def)
	}
}

func TestShellRegisterOffersSavedContacts(t *testing.T) {
	api := contactAPI(t)
	s := testShell(t, api, 100)
	c := s.childApp()
	if err := c.saveContacts([]config.Contact{{Handle: "D-1", Name: "Ada Lovelace"}, {Handle: "D-2", Name: "Grace Hopper"}}, "D-2"); err != nil {
		t.Fatal(err)
	}
	feed(s, s.openRegister("brandname.net", "11.49 USD"))
	if v := screen(s); !strings.Contains(v, "D-2  Grace Hopper") {
		t.Fatalf("the default contact is not preselected:\n%s", v)
	}
	keys(s, "down", "down", "enter") // contact field → full list
	picker, ok := s.top().(*menu)
	if !ok {
		t.Fatalf("contact list not open, top = %T", s.top())
	}
	v := screen(s)
	for _, want := range []string{"D-1  Ada Lovelace", "New contact…", "Another handle…"} {
		if !strings.Contains(v, want) {
			t.Errorf("contact list missing %q:\n%s", want, v)
		}
	}
	_ = picker
	keys(s, "N", "e", "w", "enter") // filter to "New contact…" and choose it
	f, ok := s.top().(*form)
	if !ok || f.values()["contact"] != contactNew {
		t.Fatalf("back in the form with New contact…? top = %T", s.top())
	}
	keys(s, "down", "enter") // to the button
	if cf, ok := s.top().(*form); !ok || cf.fields[0].name != "first" {
		t.Fatalf("contact form not open, top = %T", s.top())
	}
	keys(s, "esc")
	if rf, ok := s.top().(*form); !ok || rf != f {
		t.Errorf("Esc should go back to the registration form, top = %T", s.top())
	}
}

func TestShellFindsContactsOnDomains(t *testing.T) {
	api := contactAPI(t)
	s := testShell(t, api, 100)
	cmd := s.openRegister("brandname.net", "11.49 USD")
	if v := screen(s); !strings.Contains(v, "New contact…") {
		t.Fatalf("empty book should offer New contact…:\n%s", v)
	}
	feed(s, cmd)
	if v := screen(s); !strings.Contains(v, "D-1  Ada Lovelace") {
		t.Errorf("contact found on the domains is not offered:\n%s", v)
	}
}
