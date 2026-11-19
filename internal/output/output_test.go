package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/domainreseller/cli/internal/dna"
)

func render(t *testing.T, data string, opts Options) string {
	t.Helper()
	var b bytes.Buffer
	if err := Render(&b, []byte(data), opts); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestFormats(t *testing.T) {
	data := `{"totalCount":5,"items":[{"domainName":"a.com","status":0,"expirationDate":"2027-01-02T00:00:00","lockStatus":true},{"domainName":"b.com","status":7,"expirationDate":"2027-03-04T10:20:30Z","lockStatus":false}]}`
	enum, _ := dna.LookupEnum("DomainStatus")
	table := render(t, data, Options{Format: Table, Columns: []string{"domainName", "status", "expirationDate", "lockStatus"}, Enums: map[string]*dna.EnumInfo{"status": enum}})
	for _, want := range []string{"DOMAIN NAME", "Active", "PendingDelete", "2027-01-02", "2027-03-04", "✓ on", "Showing 2 of 5"} {
		if !strings.Contains(table, want) {
			t.Errorf("table missing %q:\n%s", want, table)
		}
	}
	jsonl := render(t, data, Options{Format: JSONL})
	if lines := strings.Split(strings.TrimSpace(jsonl), "\n"); len(lines) != 2 || !strings.HasPrefix(lines[0], `{"domainName":"a.com"`) {
		t.Errorf("jsonl = %q", jsonl)
	}
	if got := render(t, data, Options{Format: JSON, JQ: ".items[0].status"}); got != "0\n" {
		t.Errorf("jq number = %q", got)
	}
	if got := render(t, data, Options{Format: JSON, JQ: ".items[].domainName"}); got != "a.com\nb.com\n" {
		t.Errorf("jq strings = %q", got)
	}
	if got := render(t, "", Options{Format: JSON}); got != "{\"success\":true}\n" {
		t.Errorf("empty = %q", got)
	}
	if got := render(t, `{"a":1}`, Options{Format: Raw}); got != "{\"a\":1}\n" {
		t.Errorf("raw = %q", got)
	}
	var b bytes.Buffer
	if err := Render(&b, []byte(`{}`), Options{Format: JSON, JQ: ".["}); err == nil {
		t.Error("bad jq accepted")
	}
}

func TestObjectView(t *testing.T) {
	out := render(t, `{"domainName":"a.com","nameservers":["ns1","ns2"],"contacts":[{"firstName":"Ada","contactType":"Registrant"}],"info":{"price":9.5}}`, Options{Format: Table})
	for _, want := range []string{"a.com", "Nameservers  ns1, ns2", "Contacts", "CONTACT TYPE", "Ada", "Info", "price  9.50"} {
		if !strings.Contains(out, want) {
			t.Errorf("object view missing %q:\n%s", want, out)
		}
	}
}

func TestParseFormat(t *testing.T) {
	if f, err := ParseFormat("JSON"); err != nil || f != JSON {
		t.Error("ParseFormat JSON")
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Error("xml accepted")
	}
}
