package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/domainreseller/cli/internal/config"
	"github.com/domainreseller/cli/internal/ops"
	"github.com/domainreseller/cli/internal/ui"
)

// Contacts without the chores. The API cannot list an account's contact
// handles, so dna keeps a contact book per profile: handles it created,
// handles it saw in `domain info` and `contact get`, and the ones
// `dna contact list --scan` finds on the account's domains. One of them is
// the default, which `dna domain register` uses for all four roles when no
// --handle is given. A person at a terminal without any contact is asked to
// pick one or create one on the spot; scripts and agents get a clear error.

// noContactHelp is the error for a registration without any contact.
const noContactHelp = "no contact handle for the registration: pass --handle D-123 (used for all four roles), " +
	"or set a default once with `dna contact default D-123`.\n" +
	"No handle yet? `dna contact create` makes one and saves it as the default; " +
	"`dna contact list --scan` finds the handles on your existing domains."

// ---------------------------------------------------------------- book

type contactBook struct {
	profile, env string
	contacts     []config.Contact
	def          string
}

func (a *App) contactBook() (*contactBook, error) {
	f, err := a.config()
	if err != nil {
		return nil, err
	}
	r, err := a.resolved()
	if err != nil {
		return nil, err
	}
	cs, def := f.Contacts(r.Profile)
	return &contactBook{profile: r.Profile, env: string(r.Environment), contacts: cs, def: def}, nil
}

func (b *contactBook) find(handle string) *config.Contact {
	for i := range b.contacts {
		if strings.EqualFold(b.contacts[i].Handle, handle) {
			return &b.contacts[i]
		}
	}
	return nil
}

// saveContacts adds contacts to the current profile's book and, when
// setDefault is not empty, makes that handle the default.
func (a *App) saveContacts(cs []config.Contact, setDefault string) error {
	f, err := a.config()
	if err != nil {
		return err
	}
	r, err := a.resolved()
	if err != nil {
		return err
	}
	changed := false
	for _, c := range cs {
		if f.RememberContact(r.Profile, string(r.Environment), c) {
			changed = true
		}
	}
	if setDefault != "" {
		if _, def := f.Contacts(r.Profile); !strings.EqualFold(def, setDefault) {
			f.SetDefaultContact(r.Profile, string(r.Environment), setDefault)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return f.Save()
}

// contactFrom reads a contact as the API returns it (or as dna sends it).
func contactFrom(m map[string]any) config.Contact {
	s := func(k string) string { v, _ := m[k].(string); return strings.TrimSpace(v) }
	name := strings.TrimSpace(s("firstName") + " " + s("lastName"))
	if name == "" {
		name = s("companyName")
	}
	return config.Contact{Handle: s("handle"), Name: name, Email: s("eMail"), Country: strings.ToUpper(s("country"))}
}

func contactLabel(p ui.Painter, c config.Contact) string {
	out := p.Bold(c.Handle)
	if c.Name != "" {
		out += "  " + c.Name
	}
	if c.Email != "" {
		out += p.Muted("  " + c.Email)
	}
	return out
}

func allRoles(handle string) []any {
	out := make([]any, 0, 4)
	for _, r := range []string{"Registrant", "Admin", "Tech", "Billing"} {
		out = append(out, map[string]any{"contactType": r, "handle": handle})
	}
	return out
}

// ---------------------------------------------------------------- phone and country

// callingCodes maps ISO 3166 country codes to international calling codes.
var callingCodes = map[string]string{
	"AD": "376", "AE": "971", "AF": "93", "AG": "1", "AI": "1", "AL": "355", "AM": "374", "AO": "244", "AR": "54", "AS": "1",
	"AT": "43", "AU": "61", "AW": "297", "AX": "358", "AZ": "994", "BA": "387", "BB": "1", "BD": "880", "BE": "32", "BF": "226",
	"BG": "359", "BH": "973", "BI": "257", "BJ": "229", "BM": "1", "BN": "673", "BO": "591", "BR": "55", "BS": "1", "BT": "975",
	"BW": "267", "BY": "375", "BZ": "501", "CA": "1", "CD": "243", "CF": "236", "CG": "242", "CH": "41", "CI": "225", "CK": "682",
	"CL": "56", "CM": "237", "CN": "86", "CO": "57", "CR": "506", "CU": "53", "CV": "238", "CW": "599", "CY": "357", "CZ": "420",
	"DE": "49", "DJ": "253", "DK": "45", "DM": "1", "DO": "1", "DZ": "213", "EC": "593", "EE": "372", "EG": "20", "ER": "291",
	"ES": "34", "ET": "251", "FI": "358", "FJ": "679", "FK": "500", "FM": "691", "FO": "298", "FR": "33", "GA": "241", "GB": "44",
	"GD": "1", "GE": "995", "GF": "594", "GG": "44", "GH": "233", "GI": "350", "GL": "299", "GM": "220", "GN": "224", "GP": "590",
	"GQ": "240", "GR": "30", "GT": "502", "GU": "1", "GW": "245", "GY": "592", "HK": "852", "HN": "504", "HR": "385", "HT": "509",
	"HU": "36", "ID": "62", "IE": "353", "IL": "972", "IM": "44", "IN": "91", "IQ": "964", "IR": "98", "IS": "354", "IT": "39",
	"JE": "44", "JM": "1", "JO": "962", "JP": "81", "KE": "254", "KG": "996", "KH": "855", "KI": "686", "KM": "269", "KN": "1",
	"KP": "850", "KR": "82", "KW": "965", "KY": "1", "KZ": "7", "LA": "856", "LB": "961", "LC": "1", "LI": "423", "LK": "94",
	"LR": "231", "LS": "266", "LT": "370", "LU": "352", "LV": "371", "LY": "218", "MA": "212", "MC": "377", "MD": "373", "ME": "382",
	"MG": "261", "MH": "692", "MK": "389", "ML": "223", "MM": "95", "MN": "976", "MO": "853", "MP": "1", "MQ": "596", "MR": "222",
	"MS": "1", "MT": "356", "MU": "230", "MV": "960", "MW": "265", "MX": "52", "MY": "60", "MZ": "258", "NA": "264", "NC": "687",
	"NE": "227", "NG": "234", "NI": "505", "NL": "31", "NO": "47", "NP": "977", "NR": "674", "NU": "683", "NZ": "64", "OM": "968",
	"PA": "507", "PE": "51", "PF": "689", "PG": "675", "PH": "63", "PK": "92", "PL": "48", "PM": "508", "PR": "1", "PS": "970",
	"PT": "351", "PW": "680", "PY": "595", "QA": "974", "RE": "262", "RO": "40", "RS": "381", "RU": "7", "RW": "250", "SA": "966",
	"SB": "677", "SC": "248", "SD": "249", "SE": "46", "SG": "65", "SI": "386", "SK": "421", "SL": "232", "SM": "378", "SN": "221",
	"SO": "252", "SR": "597", "SS": "211", "ST": "239", "SV": "503", "SX": "1", "SY": "963", "SZ": "268", "TC": "1", "TD": "235",
	"TG": "228", "TH": "66", "TJ": "992", "TL": "670", "TM": "993", "TN": "216", "TO": "676", "TR": "90", "TT": "1", "TV": "688",
	"TW": "886", "TZ": "255", "UA": "380", "UG": "256", "US": "1", "UY": "598", "UZ": "998", "VA": "39", "VC": "1", "VE": "58",
	"VG": "1", "VI": "1", "VN": "84", "VU": "678", "WF": "681", "WS": "685", "XK": "383", "YE": "967", "YT": "262", "ZA": "27",
	"ZM": "260", "ZW": "263",
}

// countryOfCode maps a calling code back to its country when only one
// country uses it (+90 is Türkiye; +1 and +44 stay ambiguous).
var countryOfCode = func() map[string]string {
	seen := map[string][]string{}
	for c, code := range callingCodes {
		seen[code] = append(seen[code], c)
	}
	out := map[string]string{}
	for code, cs := range seen {
		if len(cs) == 1 {
			out[code] = cs[0]
		}
	}
	return out
}()

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitPhone turns a phone number as people write it ("+90 555 123 45 67",
// "0090 555…", or "0555 123 45 67" with a known country) into the calling
// code and the national number, digits only.
func splitPhone(raw, code, country string) (string, string, error) {
	s := strings.TrimSpace(raw)
	code = digitsOnly(code)
	if strings.HasPrefix(s, "00") {
		s = "+" + s[2:]
	}
	if strings.HasPrefix(s, "+") {
		d := digitsOnly(s)
		for n := 3; n >= 1; n-- {
			if len(d) > n && countryKnowsCode(d[:n]) {
				return d[:n], d[n:], checkNational(d[n:])
			}
		}
		return "", "", fmt.Errorf("phone %q: unknown country calling code", raw)
	}
	if code == "" {
		code = callingCodes[strings.ToUpper(strings.TrimSpace(country))]
	}
	if code == "" {
		return "", "", fmt.Errorf("phone %q: start it with the country code, e.g. +90 555 123 4567", raw)
	}
	d := digitsOnly(s)
	if code != "39" { // Italian numbers keep their leading 0
		d = strings.TrimLeft(d, "0")
	}
	return code, d, checkNational(d)
}

func countryKnowsCode(code string) bool {
	for _, c := range callingCodes {
		if c == code {
			return true
		}
	}
	return false
}

func checkNational(d string) error {
	if len(d) < 4 || len(d) > 14 {
		return fmt.Errorf("phone number %q looks too short or too long", d)
	}
	return nil
}

// normalizeContact cleans the contact fields people commonly type in other
// shapes: a full phone number with its country code, a lower-case country.
// A missing country is taken from the phone's calling code when only one
// country uses it.
func normalizeContact(m map[string]any) error {
	str := func(k string) string { v, _ := m[k].(string); return strings.TrimSpace(v) }
	if c := str("country"); c != "" {
		m["country"] = strings.ToUpper(c)
	}
	if e := str("eMail"); e != "" {
		m["eMail"] = e
	}
	if raw := str("phone"); raw != "" {
		code, num, err := splitPhone(raw, str("phoneCountryCode"), str("country"))
		if err != nil {
			return err
		}
		m["phoneCountryCode"], m["phone"] = code, num
		if str("country") == "" && countryOfCode[code] != "" {
			m["country"] = countryOfCode[code]
		}
	} else if code := str("phoneCountryCode"); code != "" {
		m["phoneCountryCode"] = digitsOnly(code)
	}
	return nil
}

// ---------------------------------------------------------------- questions

// stdin is the one buffered reader for every question this App asks, so
// answers typed ahead are never lost between questions.
func (a *App) stdin() *bufio.Reader {
	if a.inR == nil {
		a.inR = bufio.NewReader(a.In)
	}
	return a.inR
}

// ask prints a question on stderr and reads the answer. An empty answer
// takes def; check may rewrite the answer or reject it (asked again).
func (a *App) ask(label, def string, check func(string) (string, error)) (string, error) {
	p := a.errP()
	for {
		q := "  " + p.Bold(label)
		if def != "" {
			q += p.Muted(" [" + def + "]")
		}
		fmt.Fprint(a.Err, q+p.Muted(": "))
		line, rerr := a.stdin().ReadString('\n')
		v := strings.TrimSpace(line)
		if rerr != nil && v == "" && def == "" {
			fmt.Fprintln(a.Err)
			return "", usagef("canceled: no answer for %q (nothing was sent)", label)
		}
		if v == "" {
			v = def
		}
		if check != nil {
			nv, err := check(v)
			if err != nil {
				fmt.Fprintln(a.Err, "    "+p.Fg(ui.Red, "✗ ")+err.Error())
				if rerr != nil {
					return "", usageErr(err)
				}
				continue
			}
			v = nv
		}
		return v, nil
	}
}

func atLeast(n int, what string) func(string) (string, error) {
	return func(v string) (string, error) {
		if len([]rune(v)) < n {
			return "", fmt.Errorf("%s needs at least %d characters", what, n)
		}
		return v, nil
	}
}

func checkEmail(v string) (string, error) {
	at := strings.LastIndex(v, "@")
	if at < 1 || !strings.Contains(v[at:], ".") || strings.ContainsAny(v, " ,;") {
		return "", errors.New("that does not look like an e-mail address")
	}
	return v, nil
}

func checkCountry(v string) (string, error) {
	v = strings.ToUpper(strings.TrimSpace(v))
	if _, ok := callingCodes[v]; !ok {
		return "", errors.New("use the 2-letter country code, e.g. TR, DE, US")
	}
	return v, nil
}

// askContact asks for the contact fields m does not have yet, in the order
// people fill a form, and returns the completed contact.
func (a *App) askContact(m map[string]any) (map[string]any, error) {
	if m == nil {
		m = map[string]any{}
	}
	has := func(k string) bool { v, _ := m[k].(string); return strings.TrimSpace(v) != "" }
	p := a.errP()
	fmt.Fprintln(a.Err, "\n"+p.BoldFg(ui.Accent, "New contact")+p.Muted("  used as the registrant, admin, tech and billing contact"))
	type q struct {
		key, label string
		def        func() string
		check      func(string) (string, error)
	}
	none := func() string { return "" }
	questions := []q{
		{"firstName", "First name", none, atLeast(2, "the first name")},
		{"lastName", "Last name", none, atLeast(2, "the last name")},
		{"eMail", "E-mail", none, checkEmail},
		{"phone", "Phone, with country code", none, func(v string) (string, error) {
			c, _ := m["country"].(string)
			code, num, err := splitPhone(v, "", c)
			if err != nil {
				return "", err
			}
			m["phoneCountryCode"] = code
			return num, nil
		}},
		{"country", "Country (2 letters)", func() string { c, _ := m["phoneCountryCode"].(string); return countryOfCode[c] }, checkCountry},
		{"city", "City", none, atLeast(2, "the city")},
		{"state", "State or province", func() string { c, _ := m["city"].(string); return c }, atLeast(2, "the state or province")},
		{"address", "Street address", none, atLeast(3, "the address")},
		{"postalCode", "Postal code", none, atLeast(2, "the postal code")},
	}
	for _, x := range questions {
		if has(x.key) {
			continue
		}
		v, err := a.ask(x.label, x.def(), x.check)
		if err != nil {
			return nil, err
		}
		m[x.key] = v
	}
	return m, nil
}

func contactMissing(m map[string]any) bool {
	for _, k := range []string{"firstName", "lastName", "eMail", "phone", "country", "city", "state", "address", "postalCode"} {
		if v, _ := m[k].(string); strings.TrimSpace(v) == "" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- operation hooks

// prepareInput fills in what dna can work out itself before an operation
// is validated and sent.
func (a *App) prepareInput(ctx context.Context, op *ops.Operation, in ops.Input) error {
	switch op.ID {
	case "contact.create", "contact.update":
		m, _ := in["contact"].(map[string]any)
		if op.ID == "contact.create" && a.interactive() && (m == nil || contactMissing(m)) {
			var err error
			if m, err = a.askContact(m); err != nil {
				return err
			}
			in["contact"] = m
		}
		if m != nil {
			if err := normalizeContact(m); err != nil {
				return usageErr(err)
			}
		}
	case "domain.register":
		return a.prepareRegister(ctx, in)
	}
	return nil
}

// prepareRegister picks the contact (the default, or asks) and the platform
// nameservers when the command line leaves them out.
func (a *App) prepareRegister(ctx context.Context, in ops.Input) error {
	if _, ok := in["nameServers"]; !ok {
		in["nameServers"] = splitCommas([]string{platformNameservers})
		a.note("Nameservers: " + platformNameservers + " (the platform's, so `dna dns` can manage the records). --ns sets your own.")
	}
	if _, ok := in["handles"]; ok {
		return nil
	}
	b, err := a.contactBook()
	if err != nil {
		return err
	}
	handle, how := b.def, "your default contact"
	if handle == "" {
		if !a.interactive() || a.flags.yes || a.flags.dryRun {
			return usagef("%s", noContactHelp)
		}
		if handle, err = a.chooseContact(ctx, b); err != nil {
			return err
		}
		how = "now your default contact"
		b, _ = a.contactBook()
	}
	in["handles"] = allRoles(handle)
	label := handle
	if c := b.find(handle); c != nil && c.Name != "" {
		label += " (" + c.Name + ")"
	}
	a.note("Contact: " + label + " for all four roles, " + how + ". --handle picks another.")
	return nil
}

// note prints a short human-only remark on stderr.
func (a *App) note(msg string) {
	if a.machineOutput() {
		return
	}
	fmt.Fprintln(a.Err, a.errP().Muted("  "+msg))
}

// afterOperation keeps the contact book up to date from what the API said.
func (a *App) afterOperation(op *ops.Operation, in ops.Input, raw []byte) {
	switch op.ID {
	case "contact.create":
		var r struct {
			Handle string `json:"handle"`
		}
		if json.Unmarshal(raw, &r) != nil || r.Handle == "" {
			return
		}
		m, _ := in["contact"].(map[string]any)
		c := contactFrom(m)
		c.Handle = r.Handle
		b, err := a.contactBook()
		if err != nil {
			return
		}
		def := ""
		if b.def == "" {
			def = c.Handle
		}
		if a.saveContacts([]config.Contact{c}, def) != nil {
			return
		}
		if def != "" {
			a.note("Saved " + c.Handle + " as your default contact: `dna domain register` uses it when you leave out --handle.")
		} else {
			a.note("Saved " + c.Handle + " to your contacts (`dna contact list`); `dna contact default " + c.Handle + "` makes it the default.")
		}
	case "contact.get":
		var r struct {
			Contact map[string]any `json:"contact"`
		}
		if json.Unmarshal(raw, &r) == nil && r.Contact != nil {
			c := contactFrom(r.Contact)
			if c.Handle == "" {
				c.Handle, _ = in["handleCode"].(string)
			}
			_ = a.saveContacts([]config.Contact{c}, "")
		}
	case "domain.info":
		var r struct {
			Contacts []map[string]any `json:"contacts"`
		}
		if json.Unmarshal(raw, &r) == nil {
			_ = a.saveContacts(uniqueContacts(r.Contacts), "")
		}
	}
}

func uniqueContacts(ms []map[string]any) []config.Contact {
	seen := map[string]bool{}
	var out []config.Contact
	for _, m := range ms {
		c := contactFrom(m)
		if c.Handle == "" || seen[strings.ToUpper(c.Handle)] {
			continue
		}
		seen[strings.ToUpper(c.Handle)] = true
		out = append(out, c)
	}
	return out
}

// ---------------------------------------------------------------- choosing a contact

// chooseContact asks a person which contact a registration should use when
// there is no default: a saved one, a new one made on the spot, the ones on
// their domains, or a handle they type. The choice becomes the default.
func (a *App) chooseContact(ctx context.Context, b *contactBook) (string, error) {
	p := a.errP()
	for {
		if len(b.contacts) > 0 {
			fmt.Fprintln(a.Err, "\n"+p.Bold("Which contact should the domain use?")+p.Muted("  (registrant, admin, tech and billing)"))
			for i, c := range b.contacts {
				fmt.Fprintf(a.Err, "  %s  %s\n", p.BoldFg(ui.Accent, fmt.Sprintf("%2d", i+1)), contactLabel(p, c))
			}
			fmt.Fprintf(a.Err, "  %s  %s\n", p.BoldFg(ui.Accent, " n"), "Create a new contact")
			ans, err := a.ask("Number, n, or a handle", "1", nil)
			if err != nil {
				return "", err
			}
			if strings.EqualFold(ans, "n") {
				return a.createContactNow(ctx)
			}
			if n, err := strconv.Atoi(ans); err == nil && n >= 1 && n <= len(b.contacts) {
				h := b.contacts[n-1].Handle
				return h, a.saveContacts(nil, h)
			}
			return a.useHandle(ctx, ans)
		}
		fmt.Fprintln(a.Err, "\n"+p.Bold("No saved contacts yet.")+p.Muted("  A domain needs one for its registrant, admin, tech and billing roles."))
		fmt.Fprintf(a.Err, "  %s  Create a new contact now\n", p.BoldFg(ui.Accent, "1"))
		fmt.Fprintf(a.Err, "  %s  Find the contacts on my domains\n", p.BoldFg(ui.Accent, "2"))
		fmt.Fprintf(a.Err, "  %s  Type an existing handle\n", p.BoldFg(ui.Accent, "3"))
		ans, err := a.ask("Choose", "1", func(v string) (string, error) {
			if v != "1" && v != "2" && v != "3" {
				return "", errors.New("type 1, 2 or 3")
			}
			return v, nil
		})
		if err != nil {
			return "", err
		}
		switch ans {
		case "1":
			return a.createContactNow(ctx)
		case "3":
			h, err := a.ask("Handle", "", atLeast(3, "the handle"))
			if err != nil {
				return "", err
			}
			return a.useHandle(ctx, h)
		}
		found, err := a.scanContacts(ctx, 0)
		if err != nil {
			return "", err
		}
		if len(found) == 0 {
			fmt.Fprintln(a.Err, p.Muted("  None of your domains has a contact yet; let's create one."))
			return a.createContactNow(ctx)
		}
		if b, err = a.contactBook(); err != nil {
			return "", err
		}
	}
}

// useHandle checks a typed handle with the API, saves it and makes it the
// default.
func (a *App) useHandle(ctx context.Context, handle string) (string, error) {
	handle = strings.TrimSpace(handle)
	op, _ := ops.Lookup("contact.get")
	req, err := op.BuildRequest(ops.Input{"handleCode": handle})
	if err != nil {
		return "", usageErr(err)
	}
	client, err := a.client()
	if err != nil {
		return "", err
	}
	var r struct {
		Contact map[string]any `json:"contact"`
	}
	_, stop := a.spin("Checking " + handle)
	_, err = client.Do(ctx, req, &r)
	stop()
	if err != nil {
		return "", err
	}
	c := contactFrom(r.Contact)
	if c.Handle == "" {
		c.Handle = handle
	}
	if err := a.saveContacts([]config.Contact{c}, c.Handle); err != nil {
		return "", err
	}
	return c.Handle, nil
}

// createContactNow asks for a new contact's details, creates it and makes
// it the default.
func (a *App) createContactNow(ctx context.Context) (string, error) {
	res, err := a.resolved()
	if err != nil {
		return "", err
	}
	op, _ := ops.Lookup("contact.create")
	if res.ReadOnly {
		return "", &PolicyError{Operation: op.ID, Source: res.ReadOnlySource}
	}
	m, err := a.askContact(nil)
	if err != nil {
		return "", err
	}
	if err := normalizeContact(m); err != nil {
		return "", usageErr(err)
	}
	in := ops.Input{"contact": m}
	req, err := op.BuildRequest(in)
	if err != nil {
		return "", explainPositional(op, err)
	}
	client, err := a.client()
	if err != nil {
		return "", err
	}
	var raw json.RawMessage
	_, stop := a.spin("Creating the contact")
	_, err = client.Do(ctx, req, &raw)
	stop()
	if err != nil {
		return "", err
	}
	var r struct {
		Handle string `json:"handle"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Handle == "" {
		return "", fmt.Errorf("the API created no contact handle: %s", strings.TrimSpace(string(raw)))
	}
	c := contactFrom(m)
	c.Handle = r.Handle
	if err := a.saveContacts([]config.Contact{c}, c.Handle); err != nil {
		return "", err
	}
	p := a.errP()
	fmt.Fprintln(a.Err, "  "+p.BoldFg(ui.Green, "✓")+" Contact "+p.Bold(c.Handle)+" created and saved as your default contact.")
	return c.Handle, nil
}

// scanContacts reads the contacts of the account's domains (one request per
// domain, at most limit domains when limit > 0) and saves them. When the
// book had no default and every domain uses the same handle, that handle
// becomes the default.
func (a *App) scanContacts(ctx context.Context, limit int) ([]config.Contact, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	list, _ := ops.Lookup("domain.list")
	req, err := list.BuildRequest(ops.Input{})
	if err != nil {
		return nil, err
	}
	update, stop := a.spin("Listing your domains")
	defer stop()
	data, err := a.fetchAll(ctx, client, req, nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			DomainName string `json:"domainName"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		if it.DomainName != "" {
			names = append(names, it.DomainName)
		}
	}
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}
	info, _ := ops.Lookup("domain.info")
	var all []map[string]any
	for i, d := range names {
		update(fmt.Sprintf("Reading the contacts of your domains · %d of %d", i+1, len(names)))
		req, err := info.BuildRequest(ops.Input{"domainName": d})
		if err != nil {
			return nil, err
		}
		var r struct {
			Contacts []map[string]any `json:"contacts"`
		}
		if _, err := client.Do(ctx, req, &r); err != nil {
			return nil, err
		}
		all = append(all, r.Contacts...)
	}
	found := uniqueContacts(all)
	b, err := a.contactBook()
	if err != nil {
		return nil, err
	}
	def := ""
	if b.def == "" && len(found) == 1 {
		def = found[0].Handle
	}
	return found, a.saveContacts(found, def)
}

// ---------------------------------------------------------------- commands

// contactCommands are the contact book commands next to the API's contact
// operations. They work on this computer only (the API has no list call).
func (a *App) contactCommands() []*cobra.Command {
	local := map[string]string{"local": "contacts"}

	list := &cobra.Command{
		Use:         "list",
		Short:       "List the contact handles dna knows for this account",
		Annotations: local,
		Long: "Lists the contact handles saved for the current profile: the ones `dna contact create` made, the ones seen in " +
			"`dna domain info` and `dna contact get`, and, with --scan, the ones on every domain of the account " +
			"(the API has no call that lists contacts, so dna keeps this book on this computer).\n\n" +
			"The default contact is what `dna domain register` uses for all four roles when you leave out --handle.",
		Example: "  dna contact list\n  dna contact list --scan          # read the contacts of every domain and save them\n  dna contact list -o json --jq '.contacts[].handle'",
		Args:    cobra.NoArgs,
	}
	scan := list.Flags().Bool("scan", false, "Also read the contacts of your domains (one request per domain) and save them")
	limit := list.Flags().Int("limit", 0, "With --scan, read at most this many domains (0 = all)")
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		if *scan {
			if _, err := a.scanContacts(cmd.Context(), *limit); err != nil {
				return err
			}
		}
		b, err := a.contactBook()
		if err != nil {
			return err
		}
		return a.printContacts(b)
	}

	def := &cobra.Command{
		Use:         "default [handle]",
		Short:       "Show or set the contact `dna domain register` uses by default",
		Annotations: local,
		Long: "Without an argument, shows the default contact of the current profile. With a handle, checks it with the API, " +
			"saves it and makes it the default: `dna domain register` then uses it for all four roles when you leave out --handle.",
		Example: "  dna contact default\n  dna contact default D-123456789",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if _, err := a.useHandle(cmd.Context(), args[0]); err != nil {
					return err
				}
			}
			b, err := a.contactBook()
			if err != nil {
				return err
			}
			if a.machineOutput() || a.flags.jq != "" {
				out := map[string]any{"profile": b.profile, "default": b.def}
				if c := b.find(b.def); c != nil {
					out["contact"] = c
				}
				return a.printJSON(out)
			}
			p := a.outP()
			if b.def == "" {
				fmt.Fprintln(a.Out, "  No default contact for profile "+b.profile+" yet.")
				fmt.Fprintln(a.Out, p.Muted("  Set one: ")+p.Command("dna contact default D-123")+p.Muted("  ·  create one: ")+p.Command("dna contact create"))
				return nil
			}
			c := b.find(b.def)
			if c == nil {
				c = &config.Contact{Handle: b.def}
			}
			mark := "Default contact"
			if len(args) == 1 {
				mark = p.BoldFg(ui.Green, "✓") + " Default contact is now"
			}
			fmt.Fprintln(a.Out, "  "+mark+"  "+contactLabel(p, *c)+p.Muted("  (profile "+b.profile+")"))
			return nil
		},
	}

	forget := &cobra.Command{
		Use:         "forget <handle>",
		Short:       "Remove a handle from the contacts dna keeps on this computer",
		Annotations: local,
		Long:        "Removes a handle from the current profile's contact book (and as the default). The contact itself stays at the registry; nothing is sent to the API.",
		Example:     "  dna contact forget D-123456789",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := a.config()
			if err != nil {
				return err
			}
			r, err := a.resolved()
			if err != nil {
				return err
			}
			if !f.ForgetContact(r.Profile, args[0]) {
				return usagef("%s is not in the saved contacts of profile %s (see `dna contact list`)", args[0], r.Profile)
			}
			if err := f.Save(); err != nil {
				return err
			}
			if a.machineOutput() || a.flags.jq != "" {
				return a.printJSON(map[string]any{"profile": r.Profile, "forgotten": args[0]})
			}
			fmt.Fprintln(a.Out, "  "+a.outP().BoldFg(ui.Green, "✓")+" Removed "+args[0]+" from your saved contacts (the handle still exists at the registry).")
			return nil
		},
	}
	return []*cobra.Command{list, def, forget}
}

func (a *App) printContacts(b *contactBook) error {
	if a.machineOutput() || a.flags.jq != "" {
		type row struct {
			config.Contact
			Default bool `json:"default"`
		}
		rows := make([]row, 0, len(b.contacts))
		for _, c := range b.contacts {
			rows = append(rows, row{c, strings.EqualFold(c.Handle, b.def)})
		}
		return a.printJSON(map[string]any{"profile": b.profile, "default": b.def, "contacts": rows})
	}
	p := a.outP()
	if len(b.contacts) == 0 {
		fmt.Fprintln(a.Out, "  No saved contacts for profile "+b.profile+" yet.")
		fmt.Fprintln(a.Out, p.Muted("  Create one (dna asks for the details): ")+p.Command("dna contact create"))
		fmt.Fprintln(a.Out, p.Muted("  Find the ones on your domains:          ")+p.Command("dna contact list --scan"))
		return nil
	}
	hw, nw, ew := 0, 0, 0
	for _, c := range b.contacts {
		hw, nw, ew = max(hw, ui.Width(c.Handle)), max(nw, ui.Width(c.Name)), max(ew, ui.Width(c.Email))
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, "  "+p.Bold("Contacts")+p.Muted(fmt.Sprintf(" · profile %s · %d saved", b.profile, len(b.contacts))))
	fmt.Fprintln(a.Out)
	sorted := append([]config.Contact(nil), b.contacts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.EqualFold(sorted[i].Handle, b.def) && !strings.EqualFold(sorted[j].Handle, b.def)
	})
	for _, c := range sorted {
		mark, tail := "  ", ""
		if strings.EqualFold(c.Handle, b.def) {
			mark, tail = p.BoldFg(ui.Green, "★ "), p.Fg(ui.Green, "  default")
		}
		fmt.Fprintln(a.Out, "  "+mark+p.Bold(ui.PadRight(c.Handle, hw))+"  "+ui.PadRight(c.Name, nw)+"  "+p.Muted(ui.PadRight(c.Email, ew)+"  "+c.Country)+tail)
	}
	fmt.Fprintln(a.Out)
	if b.def == "" {
		fmt.Fprintln(a.Out, p.Muted("  No default yet: ")+p.Command("dna contact default "+sorted[0].Handle)+p.Muted(" lets `dna domain register` skip --handle."))
	} else {
		fmt.Fprintln(a.Out, p.Muted("  `dna domain register` uses the default when you leave out --handle."))
	}
	return nil
}
