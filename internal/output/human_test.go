package output

import (
	"strings"
	"testing"
	"time"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ui"
)

var (
	refNow          = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	domainStatus, _ = dna.LookupEnum("DomainStatus")
)

func TestSearchView(t *testing.T) {
	free := render(t, `{"success":true,"info":{"domainName":"example.com","status":"AVAILABLE","price":9.99,"currency":"USD","period":1,"isPremium":false}}`,
		Options{Format: Table, View: "domain.search"})
	for _, want := range []string{"✓ example.com is available", "9.99 USD / 1 year", "dna domain register example.com"} {
		if !strings.Contains(free, want) {
			t.Errorf("available view missing %q:\n%s", want, free)
		}
	}
	taken := render(t, `{"success":true,"info":{"domainName":"google.com","status":"NOTAVAILABLE"}}`, Options{Format: Table, View: "domain.search"})
	if !strings.Contains(taken, "✗ google.com is taken") || !strings.Contains(taken, "transfer-check") {
		t.Errorf("taken view:\n%s", taken)
	}
	// An unexpected shape falls back to the generic card.
	if out := render(t, `{"foo":1}`, Options{Format: Table, View: "domain.search"}); !strings.Contains(out, "Foo") {
		t.Errorf("fallback:\n%s", out)
	}
}

func TestBulkSearchView(t *testing.T) {
	out := render(t, `{"infos":[{"domainName":"a.com","status":"AVAILABLE","price":10,"currency":"USD"},{"domainName":"b.com","status":"NOTAVAILABLE"}]}`,
		Options{Format: Table, View: "domain.bulk-search"})
	for _, want := range []string{"✓ available", "✗ taken", "10.00 USD", "1 of 2 available", "dna domain register a.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("bulk view missing %q:\n%s", want, out)
		}
	}
}

func TestDomainListView(t *testing.T) {
	out := render(t, `{"totalCount":3,"items":[
		{"domainName":"soon.com","status":0,"expirationDate":"2026-12-01T00:00:00","lockStatus":true},
		{"domainName":"old.com","status":7,"expirationDate":"2026-11-01T00:00:00","lockStatus":false}]}`,
		Options{Format: Table, View: "domain.list", Now: refNow, Enums: map[string]*dna.EnumInfo{"status": domainStatus}})
	for _, want := range []string{"soon.com", "in 12 days", "18 days ago", "PendingDelete", "Showing 2 of 3", "1 expiring within 30 days", "1 expired", "--all"} {
		if !strings.Contains(out, want) {
			t.Errorf("list view missing %q:\n%s", want, out)
		}
	}
}

func TestDomainInfoView(t *testing.T) {
	contact := func(role string) string {
		return `{"contactType":"` + role + `","firstName":"Ada","lastName":"Lovelace","eMail":"ada@example.com","phoneCountryCode":"90","phone":"2125550101","handle":"DNA-1"}`
	}
	out := render(t, `{"domainName":"example.com","status":"Active","expirationDate":"2026-12-01T00:00:00","remainingDay":12,"lockStatus":true,"isForwarded":false,
		"nameservers":["ns1.example.net","ns2.example.net"],"hosts":[{"name":"ns1.example.com","ipAddresses":[{"ipAddress":"192.0.2.53","ipVersion":"V4"}]}],
		"contacts":[`+contact("Registrant")+`,`+contact("Admin")+`,`+contact("Tech")+`,`+contact("Billing")+`]}`,
		Options{Format: Table, View: "domain.info", Now: refNow})
	for _, want := range []string{"example.com  ● Active", "Expires", "in 12 days", "Transfer lock", "✓ on", "URL forwarding", "no\n",
		"Nameservers", "ns2.example.net", "192.0.2.53", "All roles", "Ada Lovelace", "+90 2125550101"} {
		if !strings.Contains(out, want) {
			t.Errorf("domain info missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Remaining day") {
		t.Errorf("remaining days repeat the expiry line:\n%s", out)
	}
}

func TestDNSView(t *testing.T) {
	out := render(t, `[{"name":"www","type":"A","ttl":3600,"records":[{"content":"192.0.2.1"},{"content":"192.0.2.2","disabled":true}]},{"name":"","type":"MX","ttl":300,"records":[{"content":"10 mail.example.com"}]}]`,
		Options{Format: Table, View: "dns.list", Subject: "example.com"})
	for _, want := range []string{"www", "192.0.2.2 (disabled)", "@", "10 mail.example.com", "2 record sets", "3 values", "dna dns add example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("dns view missing %q:\n%s", want, out)
		}
	}
}

func TestAccountView(t *testing.T) {
	out := render(t, `{"resellerName":"Acme","resellerGroupName":"Gold","usdBalance":1234.5,"tryBalance":-20}`, Options{Format: Table, View: "account.get"})
	for _, want := range []string{"Acme", "[Gold]", "1,234.50", "-20.00", "Balance", "USD", "TRY"} {
		if !strings.Contains(out, want) {
			t.Errorf("account view missing %q:\n%s", want, out)
		}
	}
}

func TestHumanColorsOnlyWithPainter(t *testing.T) {
	data := `{"items":[{"domainName":"a.com","status":"Active"}]}`
	if out := render(t, data, Options{Format: Table}); strings.Contains(out, "\x1b[") {
		t.Errorf("plain table has escape sequences: %q", out)
	}
	if out := render(t, data, Options{Format: Table, Painter: ui.Painter{Level: ui.TrueColor}}); !strings.Contains(out, "\x1b[") {
		t.Error("painted table has no colors")
	}
	// JSON stays byte-identical without a painter (scripts and agents).
	if out := render(t, `{"a":1}`, Options{Format: JSON}); out != "{\"a\":1}\n" {
		t.Errorf("json = %q", out)
	}
}

func TestMoney(t *testing.T) {
	for in, want := range map[float64]string{0: "0.00", 9.5: "9.50", 1234567.891: "1,234,567.89", -1000: "-1,000.00"} {
		if got := money(in); got != want {
			t.Errorf("money(%v) = %q, want %q", in, got, want)
		}
	}
}
