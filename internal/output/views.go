package output

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/domainreseller/cli/internal/ui"
)

// views are tailored human renderings for the most used operations, keyed by
// operation ID. A view returns false when the response does not have the
// expected shape; the generic table/card is used then.
var views = map[string]func(h *human, v any) (string, bool){
	"domain.search":      viewSearch,
	"domain.bulk-search": viewBulkSearch,
	"domain.list":        viewDomainList,
	"domain.info":        viewDomainInfo,
	"account.get":        viewAccount,
	"dns.list":           viewDNS,
}

// ---------------------------------------------------------------- search

func availability(status string) (icon string, tone ui.Tone, word string) {
	switch strings.ToUpper(strings.ReplaceAll(status, "_", "")) {
	case "AVAILABLE":
		return "✓", ui.Good, "available"
	case "NOTAVAILABLE":
		return "✗", ui.Bad, "taken"
	case "UNSUPPORTEDTLD":
		return "–", ui.Warn, "not offered"
	}
	if status == "" {
		return "?", ui.Warn, "unknown"
	}
	return "?", ui.Warn, strings.ToLower(status)
}

func priceText(info map[string]any) string {
	price, ok := num(info["price"])
	if !ok || price <= 0 {
		return ""
	}
	s := money(price)
	if c := str(info["currency"]); c != "" {
		s += " " + c
	}
	if period, ok := num(info["period"]); ok && period > 0 {
		years := "year"
		if period != 1 {
			years = "years"
		}
		s += " / " + strconv.Itoa(int(period)) + " " + years
	}
	return s
}

func viewSearch(h *human, v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", false
	}
	info, ok := m["info"].(map[string]any)
	if !ok {
		return "", false
	}
	p := h.p
	name := firstNonEmpty(str(info["domainName"]), h.subject)
	icon, tone, word := availability(str(info["status"]))
	var sb strings.Builder
	sb.WriteString("\n  " + p.BoldFg(tone.Color(), icon) + " " + p.Bold(name) + " " + p.Toned(tone, "is "+word) + "\n")

	var facts []string
	if s := priceText(info); s != "" {
		facts = append(facts, p.Bold(s))
	}
	if b, _ := info["isPremium"].(bool); b {
		facts = append(facts, p.Fg(ui.Amber, "premium"))
	}
	if b, _ := info["isDocumentRequired"].(bool); b {
		facts = append(facts, p.Fg(ui.Amber, "registry documents required"))
	}
	if len(facts) > 0 {
		sb.WriteString("    " + strings.Join(facts, p.Muted(" · ")) + "\n")
	}
	for _, r := range []string{str(info["reason"]), str(m["reason"]), str(m["operationMessage"])} {
		if r != "" && !strings.EqualFold(r, word) {
			sb.WriteString("    " + p.Muted(r) + "\n")
			break
		}
	}
	sb.WriteString("\n")
	switch tone {
	case ui.Good:
		sb.WriteString("  " + p.Hint(p.Muted("Register it (preview first): ")+p.Command("dna domain register "+name+" --dry-run")) + "\n\n")
	case ui.Bad:
		sb.WriteString("  " + p.Hint(p.Muted("Yours at another registrar? ")+p.Command("dna domain transfer-check "+name+" <auth-code>")) + "\n\n")
	default:
		if word == "not offered" {
			sb.WriteString("  " + p.Hint(p.Muted("Every extension and its price: ")+p.Command("dna product tlds --all")) + "\n\n")
		}
	}
	return sb.String(), true
}

func viewBulkSearch(h *human, v any) (string, bool) {
	list, ok := ListItems(v)
	if !ok {
		return "", false
	}
	p := h.p
	t := ui.Table{Headers: []string{"DOMAIN", "AVAILABILITY", "PRICE", "NOTES"}, Align: []ui.Align{ui.Left, ui.Left, ui.Right, ui.Left}}
	free := 0
	var firstFree string
	for _, r := range list {
		info, isObj := r.(map[string]any)
		if !isObj {
			return "", false
		}
		icon, tone, word := availability(str(info["status"]))
		if tone == ui.Good {
			free++
			if firstFree == "" {
				firstFree = str(info["domainName"])
			}
		}
		var notes []string
		if b, _ := info["isPremium"].(bool); b {
			notes = append(notes, "premium")
		}
		if b, _ := info["isDocumentRequired"].(bool); b {
			notes = append(notes, "documents")
		}
		if r := str(info["reason"]); r != "" && tone != ui.Good {
			notes = append(notes, r)
		}
		price := ""
		if pr, ok := num(info["price"]); ok && pr > 0 {
			price = money(pr) + " " + str(info["currency"])
		}
		tc := tone
		t.Rows = append(t.Rows, []ui.Cell{
			{Text: str(info["domainName"]), Style: p.Bold},
			{Text: icon + " " + word, Style: func(s string) string { return p.Fg(tc.Color(), s) }},
			{Text: strings.TrimSpace(price)},
			{Text: strings.Join(notes, ", "), Style: p.Muted},
		})
	}
	var sb strings.Builder
	sb.WriteString("\n" + p.Render(t, h.width))
	sb.WriteString(fmt.Sprintf("\n  %s of %d available\n", p.BoldFg(ui.Green, strconv.Itoa(free)), len(list)))
	if firstFree != "" {
		sb.WriteString("  " + p.Hint(p.Muted("Register one (preview first): ")+p.Command("dna domain register "+firstFree+" --dry-run")) + "\n")
	}
	sb.WriteString("\n")
	return sb.String(), true
}

// ---------------------------------------------------------------- domains

func viewDomainList(h *human, v any) (string, bool) {
	list, ok := ListItems(v)
	if !ok {
		return "", false
	}
	p := h.p
	if len(list) == 0 {
		return "\n  " + p.Muted("No domains found.") + "\n\n", true
	}
	t := ui.Table{Headers: []string{"DOMAIN", "STATUS", "EXPIRES", "RENEWAL", "LOCK", "PRIVACY"}}
	soon, expired := 0, 0
	for _, r := range list {
		d, isObj := r.(map[string]any)
		if !isObj {
			return "", false
		}
		status := h.cell("status", d["status"]).Cell
		exp := h.cell("expirationDate", d["expirationDate"]).Cell
		if date, _, ok := parseDate(str(d["expirationDate"])); ok && date != "" {
			n := daysUntil(date, h.now)
			if rd, ok := num(d["remainingDay"]); ok {
				n = int(rd)
			}
			switch {
			case n < 0:
				expired++
			case n <= 30:
				soon++
			}
		}
		t.Rows = append(t.Rows, []ui.Cell{
			{Text: str(d["domainName"]), Style: p.Bold},
			status,
			exp,
			h.cell("renewalMode", d["renewalMode"]).Cell,
			h.cell("lockStatus", d["lockStatus"]).Cell,
			h.cell("privacyProtectionStatus", d["privacyProtectionStatus"]).Cell,
		})
	}
	var notes []string
	if soon > 0 {
		notes = append(notes, p.Fg(ui.Amber, fmt.Sprintf("%d expiring within 30 days", soon)))
	}
	if expired > 0 {
		notes = append(notes, p.Fg(ui.Red, fmt.Sprintf("%d expired", expired)))
	}
	var sb strings.Builder
	sb.WriteString("\n" + p.Render(t, h.width))
	footer := h.listFooter(v, len(list), "domain", "domains", strings.Join(notes, p.Muted(" · ")))
	sb.WriteString(strings.TrimSuffix(footer, "\n"))
	sb.WriteString("  " + p.Hint(p.Muted("Details: ")+p.Command("dna domain info <domain>")) + "\n\n")
	return sb.String(), true
}

func viewDomainInfo(h *human, v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok || str(m["domainName"]) == "" {
		return "", false
	}
	hide := map[string]bool{"resellerId": true, "objectId": true, "isIdnName": true, "remainingDay": true}
	if str(m["domainNameIdn"]) == str(m["domainName"]) {
		hide["domainNameIdn"] = true
	}
	cp := make(map[string]any, len(m))
	for k, val := range m {
		cp[k] = val
	}
	if rows := contactRows(m["contacts"]); rows != nil {
		cp["contacts"] = rows
	}
	// Child hosts: name and IP addresses on one line.
	if list, ok := m["hosts"].([]any); ok {
		var rows []any
		for _, x := range list {
			hm, isObj := x.(map[string]any)
			if !isObj {
				rows = nil
				break
			}
			var ips []string
			if addrs, ok := hm["ipAddresses"].([]any); ok {
				for _, a := range addrs {
					if am, ok := a.(map[string]any); ok {
						ips = append(ips, str(am["ipAddress"]))
					}
				}
			}
			rows = append(rows, map[string]any{"name": hm["name"], "ipAddresses": strings.Join(ips, ", ")})
		}
		if rows != nil {
			cp["hosts"] = rows
		}
	}
	m = cp
	return h.card(m, cardSpec{
		columns: map[string][]string{
			"contacts": {"role", "name", "company", "eMail", "phone", "handle"},
			"hosts":    {"name", "ipAddresses"},
		},
		title: "domainName",
		first: []string{"expirationDate", "startDate", "updatedDate", "transferDate", "renewalMode", "lockStatus",
			"privacyProtectionStatus", "isForwarded", "useTrusteeContact", "authCode", "statusCode", "statusReason", "rgpStatues",
			"nameservers", "hosts", "contacts"},
		labels: map[string]string{
			"expirationDate": "Expires", "startDate": "Registered", "updatedDate": "Updated", "transferDate": "Transferred",
			"renewalMode": "Renewal", "lockStatus": "Transfer lock", "privacyProtectionStatus": "WHOIS privacy",
			"isForwarded": "URL forwarding", "useTrusteeContact": "Trustee contact", "authCode": "Auth code",
			"statusCode": "Registry status", "statusReason": "Status reason", "rgpStatues": "RGP status",
			"nameservers": "Nameservers", "hosts": "Child hosts", "contacts": "Contacts", "additionalAttributes": "Registry attributes",
			"domainNameIdn": "IDN",
		},
		hide:     hide,
		sections: []string{"nameservers"},
	}), true
}

// contactRows turns domain contacts into compact table rows. Identical
// contacts (often one person for every role) share a row.
func contactRows(v any) []any {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	type row struct {
		roles  []string
		fields map[string]any
	}
	var rows []*row
	index := map[string]*row{}
	for _, c := range list {
		cm, isObj := c.(map[string]any)
		if !isObj {
			return nil
		}
		name := strings.TrimSpace(str(cm["firstName"]) + " " + str(cm["lastName"]))
		phone := str(cm["phone"])
		if cc := str(cm["phoneCountryCode"]); cc != "" && phone != "" {
			phone = "+" + strings.TrimPrefix(cc, "+") + " " + phone
		}
		fields := map[string]any{"name": name, "company": str(cm["companyName"]), "eMail": str(cm["eMail"]), "phone": phone, "handle": str(cm["handle"])}
		key := fmt.Sprint(fields["name"], "|", fields["company"], "|", fields["eMail"], "|", phone, "|", fields["handle"])
		if r, ok := index[key]; ok {
			r.roles = append(r.roles, str(cm["contactType"]))
			continue
		}
		r := &row{roles: []string{str(cm["contactType"])}, fields: fields}
		index[key] = r
		rows = append(rows, r)
	}
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		role := strings.Join(r.roles, ", ")
		if len(r.roles) == 4 {
			role = "All roles"
		}
		r.fields["role"] = role
		out = append(out, r.fields)
	}
	return out
}

// ---------------------------------------------------------------- account

func viewAccount(h *human, v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", false
	}
	p := h.p
	var sb strings.Builder
	sb.WriteString("\n  " + p.BoldFg(ui.Accent, firstNonEmpty(str(m["resellerName"]), "Reseller account")))
	if g := str(m["resellerGroupName"]); g != "" {
		sb.WriteString("  " + p.Badge(ui.Navy, g))
	}
	sb.WriteString("\n\n")
	var pairs [][2]string
	width := 0
	for _, k := range []string{"usdBalance", "tryBalance"} {
		if x, ok := num(m[k]); ok {
			if w := len(money(x)); w > width {
				width = w
			}
		}
	}
	label := "Balance"
	for _, c := range []struct{ key, cur string }{{"usdBalance", "USD"}, {"tryBalance", "TRY"}} {
		x, ok := num(m[c.key])
		if !ok {
			continue
		}
		val := p.Bold(ui.PadLeft(money(x), width)) + " " + p.Muted(c.cur)
		if x < 0 {
			val = p.BoldFg(ui.Red, ui.PadLeft(money(x), width)) + " " + p.Muted(c.cur)
		}
		pairs = append(pairs, [2]string{label, val})
		label = ""
	}
	if id := str(m["resellerId"]); id != "" {
		pairs = append(pairs, [2]string{"Reseller ID", p.Muted(id)})
	}
	sb.WriteString(p.KV(pairs, 2))
	sb.WriteString("\n  " + p.Hint(p.Muted("Ledger: ")+p.Command("dna account transactions")) + "\n\n")
	return sb.String(), true
}

// ---------------------------------------------------------------- dns

func viewDNS(h *human, v any) (string, bool) {
	list, ok := ListItems(v)
	if !ok {
		return "", false
	}
	p := h.p
	domain := h.subject
	if len(list) == 0 {
		var sb strings.Builder
		sb.WriteString("\n  " + p.Muted("No DNS records yet.") + "\n\n")
		if domain != "" {
			sb.WriteString("  " + p.Hint(p.Muted("Add one: ")+p.Command("dna dns add "+domain+" --name www --type A --content 192.0.2.10")) + "\n\n")
		}
		return sb.String(), true
	}
	t := ui.Table{Headers: []string{"NAME", "TYPE", "TTL", "VALUE"}, Align: []ui.Align{ui.Left, ui.Left, ui.Right, ui.Left}}
	values := 0
	for _, r := range list {
		set, isObj := r.(map[string]any)
		if !isObj {
			return "", false
		}
		name := str(set["name"])
		if name == "" {
			name = "@"
		}
		typ := str(set["type"])
		ttl := ""
		if x, ok := num(set["ttl"]); ok {
			ttl = strconv.Itoa(int(x))
		}
		records, _ := set["records"].([]any)
		if len(records) == 0 {
			t.Rows = append(t.Rows, []ui.Cell{{Text: name, Style: p.Bold}, {Text: typ, Style: p.Accent}, {Text: ttl}, {Text: "–", Style: p.Muted}})
			continue
		}
		for i, rec := range records {
			content, disabled := "", false
			switch x := rec.(type) {
			case map[string]any:
				content = str(x["content"])
				disabled, _ = x["disabled"].(bool)
			case string:
				content = x
			}
			values++
			val := ui.Cell{Text: content}
			if disabled {
				val = ui.Cell{Text: content + " (disabled)", Style: p.Muted}
			}
			if i == 0 {
				t.Rows = append(t.Rows, []ui.Cell{{Text: name, Style: p.Bold}, {Text: typ, Style: p.Accent}, {Text: ttl}, val})
			} else {
				t.Rows = append(t.Rows, []ui.Cell{{}, {}, {}, val})
			}
		}
	}
	var sb strings.Builder
	sb.WriteString("\n" + p.Render(t, h.width))
	sets := "record sets"
	if len(list) == 1 {
		sets = "record set"
	}
	sb.WriteString(fmt.Sprintf("\n  %d %s %s %d values\n", len(list), sets, p.Muted("·"), values))
	if domain != "" {
		sb.WriteString("  " + p.Hint(p.Muted("Add a value: ")+p.Command("dna dns add "+domain+" --name <name> --type <type> --content <value>")) + "\n")
	}
	sb.WriteString("\n")
	return sb.String(), true
}
