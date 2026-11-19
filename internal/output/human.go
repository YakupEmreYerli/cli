package output

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ui"
)

// human renders API responses for people: aligned tables with colored
// statuses, cards for single objects and tailored views for the most used
// operations. It is only used for the table format.
type human struct {
	p       ui.Painter
	width   int
	now     time.Time
	enums   map[string]*dna.EnumInfo
	cols    []string
	subject string
}

func newHuman(opts Options) *human {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	return &human{p: opts.Painter, width: opts.Width, now: now, enums: opts.Enums, cols: opts.Columns, subject: opts.Subject}
}

func writeHuman(w io.Writer, v any, opts Options) error {
	h := newHuman(opts)
	if list, ok := ListItems(v); ok && opts.RowHook != nil {
		for _, r := range list {
			if m, isObj := r.(map[string]any); isObj {
				opts.RowHook(m)
			}
		}
	}
	if view, ok := views[opts.View]; ok {
		if s, handled := view(h, v); handled {
			_, err := io.WriteString(w, s)
			return err
		}
	}
	_, err := io.WriteString(w, h.generic(v))
	return err
}

func (h *human) generic(v any) string {
	if list, ok := ListItems(v); ok {
		var sb strings.Builder
		sb.WriteString("\n")
		sb.WriteString(h.table(list, h.cols))
		sb.WriteString(h.listFooter(v, len(list), "result", "results", ""))
		return sb.String()
	}
	if m, ok := v.(map[string]any); ok {
		return h.card(m, cardSpec{})
	}
	return "\n  " + h.cell("", v).render() + "\n\n"
}

// ---------------------------------------------------------------- tables

func (h *human) table(rows []any, preferred []string) string {
	if len(rows) == 0 {
		return "  " + h.p.Muted("No results.") + "\n"
	}
	if _, isObj := rows[0].(map[string]any); !isObj {
		var sb strings.Builder
		for _, r := range rows {
			sb.WriteString("  " + h.cell("", r).render() + "\n")
		}
		return sb.String()
	}
	cols := columns(rows, preferred)
	t := ui.Table{Headers: make([]string, len(cols)), Align: make([]ui.Align, len(cols))}
	for i, c := range cols {
		t.Headers[i] = strings.ToUpper(humanize(c))
		if numericColumn(rows, c, h.enums[c]) {
			t.Align[i] = ui.Right
		}
	}
	for _, r := range rows {
		m, _ := r.(map[string]any)
		cells := make([]ui.Cell, len(cols))
		for i, c := range cols {
			cells[i] = h.cell(c, m[c]).Cell
		}
		t.Rows = append(t.Rows, cells)
	}
	return h.p.Render(t, h.width)
}

func numericColumn(rows []any, key string, enum *dna.EnumInfo) bool {
	if enum != nil {
		return false
	}
	seen := false
	for _, r := range rows {
		m, _ := r.(map[string]any)
		switch m[key].(type) {
		case nil:
		case float64:
			seen = true
		default:
			return false
		}
	}
	return seen
}

func (h *human) listFooter(v any, shown int, one, many, extra string) string {
	var parts []string
	m, _ := v.(map[string]any)
	total := -1
	if m != nil {
		if t, ok := m["totalCount"].(float64); ok {
			total = int(t)
		}
	}
	if total > shown {
		parts = append(parts, fmt.Sprintf("Showing %d of %d", shown, total))
	} else if shown > 0 {
		noun := many
		if shown == 1 {
			noun = one
		}
		parts = append(parts, fmt.Sprintf("%d %s", shown, noun))
	}
	if extra != "" {
		parts = append(parts, extra)
	}
	var sb strings.Builder
	if len(parts) > 0 {
		sb.WriteString("\n  " + strings.Join(parts, h.p.Muted(" · ")) + "\n")
	}
	if total > shown {
		sb.WriteString("  " + h.p.Hint(h.p.Muted("add ")+h.p.Command("--all")+h.p.Muted(" to fetch every page")) + "\n")
	}
	if m != nil {
		for _, k := range []string{"operationMessage", "reason"} {
			if s, ok := m[k].(string); ok && s != "" {
				sb.WriteString("  " + h.p.Muted(humanize(k)+": ") + s + "\n")
			}
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// ---------------------------------------------------------------- cells

type cell struct {
	ui.Cell
}

func (c cell) render() string {
	if c.Style != nil && c.Text != "" {
		return c.Style(c.Text)
	}
	return c.Text
}

func (h *human) muted() func(string) string { return h.p.Muted }

// cell formats one value for a table or card, choosing a style from the
// key (statuses, dates, money) and the value type.
func (h *human) cell(key string, v any) cell {
	p := h.p
	switch x := v.(type) {
	case nil:
		return cell{ui.Cell{Text: "–", Style: h.muted()}}
	case bool:
		if statusKey(key) {
			if x {
				return cell{ui.Cell{Text: "✓ on", Style: func(s string) string { return p.Fg(ui.Green, s) }}}
			}
			return cell{ui.Cell{Text: "off", Style: h.muted()}}
		}
		if x {
			return cell{ui.Cell{Text: "✓", Style: func(s string) string { return p.Fg(ui.Green, s) }}}
		}
		return cell{ui.Cell{Text: "–", Style: h.muted()}}
	case float64:
		if e := h.enums[key]; e != nil && x == math.Trunc(x) {
			if name := e.NameOf(int(x)); name != "" {
				return h.statusCell(key, name)
			}
		}
		if key == "remainingDay" || key == "remainingDays" {
			d := int(x)
			return cell{ui.Cell{Text: days(d), Style: func(s string) string { return p.Toned(dayTone(d), s) }}}
		}
		if moneyKey(key) {
			return cell{ui.Cell{Text: money(x)}}
		}
		return cell{ui.Cell{Text: strconv.FormatFloat(x, 'f', -1, 64)}}
	case string:
		if x == "" {
			return cell{ui.Cell{Text: "–", Style: h.muted()}}
		}
		if d, t, ok := parseDate(x); ok {
			if d == "" {
				return cell{ui.Cell{Text: "–", Style: h.muted()}}
			}
			text := d
			if t != "" {
				text += " " + t
			}
			if expiryKey(key) {
				n := daysUntil(d, h.now)
				rel := relDays(n)
				return cell{ui.Cell{Text: d + "  " + rel, Style: func(s string) string {
					if i := strings.Index(s, "  "); i > 0 {
						return s[:i] + "  " + p.Toned(dayTone(n), s[i+2:])
					}
					return s
				}}}
			}
			return cell{ui.Cell{Text: text}}
		}
		if statusKey(key) {
			return h.statusCell(key, x)
		}
		return cell{ui.Cell{Text: strings.ReplaceAll(x, "\n", " ")}}
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				parts = append(parts, summarize(m))
				continue
			}
			parts = append(parts, h.cell(key, item).Text)
		}
		if len(parts) == 0 {
			return cell{ui.Cell{Text: "–", Style: h.muted()}}
		}
		return cell{ui.Cell{Text: strings.Join(parts, ", ")}}
	case map[string]any:
		if len(x) == 0 {
			return cell{ui.Cell{Text: "–", Style: h.muted()}}
		}
		b, _ := json.Marshal(x)
		return cell{ui.Cell{Text: string(b)}}
	}
	return cell{ui.Cell{Text: fmt.Sprint(v)}}
}

func (h *human) statusCell(key, s string) cell {
	if !statusKey(key) || ui.ToneOf(s) == ui.Neutral {
		return cell{ui.Cell{Text: s}}
	}
	return cell{ui.Cell{Text: s, Style: h.p.Status, Extra: 2}}
}

func statusKey(k string) bool {
	return k == "status" || k == "statusCode" || strings.HasSuffix(k, "Status") || k == "chargeType"
}

func expiryKey(k string) bool {
	l := strings.ToLower(k)
	return strings.Contains(l, "expir") || l == "validto" || l == "enddate" || l == "validuntil"
}

func moneyKey(k string) bool {
	l := strings.ToLower(k)
	for _, s := range []string{"price", "balance", "amount", "change", "cost", "fee", "total"} {
		if strings.Contains(l, s) && !strings.Contains(l, "count") {
			return true
		}
	}
	return false
}

// money formats an amount with two decimals and thousands separators.
func money(x float64) string {
	s := strconv.FormatFloat(math.Abs(x), 'f', 2, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	var sb strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			sb.WriteByte(',')
		}
		sb.WriteRune(r)
	}
	out := sb.String() + "." + frac
	if x < 0 {
		out = "-" + out
	}
	return out
}

// parseDate splits an API timestamp into date and time ("" for midnight).
// The zero date 0001-01-01 yields an empty date.
func parseDate(s string) (date, clock string, ok bool) {
	m := dateTime.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	if strings.HasPrefix(m[1], "0001-01-01") {
		return "", "", true
	}
	if m[2] != "00:00" {
		clock = m[2]
	}
	return m[1], clock, true
}

func daysUntil(date string, now time.Time) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(math.Round(t.Sub(today).Hours() / 24))
}

func relDays(n int) string {
	switch {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n == -1:
		return "yesterday"
	case n < 0:
		return days(-n) + " ago"
	}
	return "in " + days(n)
}

func days(n int) string {
	if n == 1 || n == -1 {
		return strconv.Itoa(n) + " day"
	}
	return strconv.Itoa(n) + " days"
}

func dayTone(n int) ui.Tone {
	switch {
	case n < 0:
		return ui.Bad
	case n <= 30:
		return ui.Warn
	}
	return ui.Neutral
}

// ---------------------------------------------------------------- cards

type cardSpec struct {
	title    string              // key of the title value
	first    []string            // keys shown first, in this order
	labels   map[string]string   // custom labels
	hide     map[string]bool     // keys never shown
	sections []string            // scalar lists shown as their own section
	columns  map[string][]string // column order of nested tables
}

var titleKeys = []string{"domainName", "name", "handle", "resellerName", "productName", "orderCode", "hostName", "commonName"}

func (h *human) card(m map[string]any, spec cardSpec) string {
	p := h.p
	var sb strings.Builder
	sb.WriteString("\n")

	titleKey := spec.title
	if titleKey == "" {
		for _, k := range titleKeys {
			if s, ok := m[k].(string); ok && s != "" {
				titleKey = k
				break
			}
		}
	}
	used := map[string]bool{}
	for k := range spec.hide {
		used[k] = true
	}
	header := ""
	if s, ok := m[titleKey].(string); ok && s != "" {
		header = p.BoldFg(ui.Accent, s)
		used[titleKey] = true
	}
	if st, ok := m["status"].(string); ok && st != "" && ui.ToneOf(st) != ui.Neutral {
		header += "  " + p.Status(st)
		used["status"] = true
	}
	if ok, isBool := m["success"].(bool); isBool {
		used["success"] = true
		msg, _ := m["message"].(string)
		if msg != "" {
			used["message"] = true
		}
		mark := p.BoldFg(ui.Green, "✓") + " " + firstNonEmpty(msg, "Done")
		if !ok {
			mark = p.BoldFg(ui.Red, "✗") + " " + firstNonEmpty(msg, "The API reported a failure")
		}
		if header != "" {
			header = mark + "  " + p.Muted("·") + "  " + header
		} else {
			header = mark
		}
	}
	if header != "" {
		sb.WriteString("  " + header + "\n\n")
	}

	keys := orderedKeys(m, spec.first)
	var pairs [][2]string
	var nested []string
	for _, k := range keys {
		if used[k] {
			continue
		}
		v := m[k]
		if isEmpty(v) {
			continue
		}
		if contains(spec.sections, k) {
			nested = append(nested, k)
			continue
		}
		switch x := v.(type) {
		case map[string]any:
			nested = append(nested, k)
			continue
		case []any:
			if _, isObj := x[0].(map[string]any); isObj {
				nested = append(nested, k)
				continue
			}
		case string:
			if strings.Contains(strings.TrimSpace(x), "\n") {
				nested = append(nested, k)
				continue
			}
		}
		label := spec.labels[k]
		if label == "" {
			label = capitalize(humanize(k))
		}
		c := h.cell(k, v)
		if b, isBool := v.(bool); isBool && !statusKey(k) {
			// In a card a lone "–" reads as "empty"; say yes or no.
			c = cell{ui.Cell{Text: "no", Style: h.muted()}}
			if b {
				c = cell{ui.Cell{Text: "✓ yes", Style: func(s string) string { return p.Fg(ui.Green, s) }}}
			}
		}
		text := c.Text
		if h.width > 0 {
			text = ui.Truncate(text, h.width-6-labelWidth(pairs, label))
		}
		c.Text = text
		pairs = append(pairs, [2]string{label, c.render()})
	}
	sb.WriteString(p.KV(pairs, 2))

	for _, k := range nested {
		label := spec.labels[k]
		if label == "" {
			label = capitalize(humanize(k))
		}
		sb.WriteString("\n  " + p.Bold(label) + "\n")
		switch x := m[k].(type) {
		case map[string]any:
			var sub [][2]string
			for _, sk := range orderedKeys(x, nil) {
				if isEmpty(x[sk]) {
					continue
				}
				sub = append(sub, [2]string{sk, h.cell(sk, x[sk]).render()})
			}
			sb.WriteString(p.KV(sub, 4))
		case []any:
			if _, isObj := x[0].(map[string]any); isObj {
				sb.WriteString(h.subTable(x, spec.columns[k]))
				continue
			}
			for _, item := range x {
				sb.WriteString("    " + h.cell(k, item).render() + "\n")
			}
		case string:
			for _, line := range strings.Split(strings.TrimRight(x, "\n"), "\n") {
				sb.WriteString("    " + p.Muted(line) + "\n")
			}
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

func (h *human) subTable(rows []any, preferred []string) string {
	cols := columns(rows, preferred)
	t := ui.Table{Indent: 4, Headers: make([]string, len(cols))}
	for i, c := range cols {
		t.Headers[i] = strings.ToUpper(humanize(c))
	}
	for _, r := range rows {
		m, _ := r.(map[string]any)
		cells := make([]ui.Cell, len(cols))
		for i, c := range cols {
			cells[i] = h.cell(c, m[c]).Cell
		}
		t.Rows = append(t.Rows, cells)
	}
	return h.p.Render(t, h.width)
}

func labelWidth(pairs [][2]string, label string) int {
	w := ui.Width(label)
	for _, kv := range pairs {
		if lw := ui.Width(kv[0]); lw > w {
			w = lw
		}
	}
	return w
}

func orderedKeys(m map[string]any, first []string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, k := range first {
		if _, ok := m[k]; ok && !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	var rest []string
	for k := range m {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		if x == "" {
			return true
		}
		if d, _, ok := parseDate(x); ok && d == "" {
			return true
		}
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}
