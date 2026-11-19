// Package output renders API responses as JSON, JSON lines, tables or raw
// bytes, and applies --jq filters. The table format is the human view
// (see human.go); every other format is for machines and never styled
// unless a Painter is given for a terminal.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/itchyny/gojq"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ui"
)

// Format is an output format.
type Format string

const (
	Auto  Format = "auto"  // table on a terminal, JSON otherwise
	JSON  Format = "json"  // the API response as JSON
	JSONL Format = "jsonl" // one JSON value per line (list items)
	Table Format = "table" // human-readable table / key-value view
	Raw   Format = "raw"   // response body exactly as received
)

// Formats lists the accepted --output values.
var Formats = []Format{Auto, JSON, JSONL, Table, Raw}

// ParseFormat validates a --output value.
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case "", Auto:
		return Auto, nil
	case JSON, JSONL, Table, Raw:
		return f, nil
	case "yaml", "yml":
		return "", fmt.Errorf("yaml output is not supported; use -o json")
	}
	return "", fmt.Errorf("unknown output format %q (use auto, json, jsonl, table or raw)", s)
}

// Options control rendering.
type Options struct {
	Format  Format // must not be Auto (resolve it first)
	JQ      string // gojq filter applied before rendering
	Pretty  bool   // indent JSON
	Columns []string
	Enums   map[string]*dna.EnumInfo // field -> enum, for table cells
	// RowHook adds derived columns to each list row in table output.
	RowHook func(row map[string]any)

	// Human view only (table format, or JSON on a terminal):
	Painter ui.Painter // colors; the zero value prints plain text
	Width   int        // terminal width for tables (0 = unlimited)
	View    string     // operation ID, selects a tailored view
	Subject string     // the main argument (usually the domain), for hints
	Now     time.Time  // reference time for "in 12 days" (zero = now)
}

// EmptySuccess is printed for successful calls that return no body.
var EmptySuccess = []byte(`{"success":true}`)

// Render writes data (a JSON document) to w.
func Render(w io.Writer, data []byte, opts Options) error {
	if len(bytes.TrimSpace(data)) == 0 {
		if opts.Format == Table && opts.JQ == "" {
			_, err := fmt.Fprint(w, "\n  "+opts.Painter.BoldFg(ui.Green, "✓")+" Done\n\n")
			return err
		}
		data = EmptySuccess
	}
	if opts.Format == Raw && opts.JQ == "" {
		_, err := w.Write(data)
		if err == nil && !bytes.HasSuffix(data, []byte("\n")) {
			_, err = w.Write([]byte("\n"))
		}
		return err
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		// Not JSON (some endpoints answer text/plain): print as is.
		_, err := fmt.Fprintln(w, strings.TrimRight(string(data), "\n"))
		return err
	}
	if opts.JQ != "" {
		return runJQ(w, v, opts)
	}
	switch opts.Format {
	case JSONL:
		return writeJSONL(w, v)
	case Table:
		return writeHuman(w, v, opts)
	}
	return writeJSON(w, v, opts.Pretty, opts.Painter)
}

// WriteJSON writes v as JSON followed by a newline.
func WriteJSON(w io.Writer, v any, pretty bool) error {
	return writeJSON(w, v, pretty, ui.Painter{})
}

// WriteJSONColor is WriteJSON with syntax colors for a terminal (the text is
// identical; a zero Painter adds nothing).
func WriteJSONColor(w io.Writer, v any, pretty bool, p ui.Painter) error {
	return writeJSON(w, v, pretty, p)
}

func writeJSON(w io.Writer, v any, pretty bool, p ui.Painter) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(p.ColorizeJSON(buf.Bytes()))
	return err
}

func runJQ(w io.Writer, v any, opts Options) error {
	q, err := gojq.Parse(opts.JQ)
	if err != nil {
		return &JQError{Expr: opts.JQ, Err: err}
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return &JQError{Expr: opts.JQ, Err: err}
	}
	iter := code.Run(normalizeNumbers(v))
	for {
		x, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, isErr := x.(error); isErr {
			if _, halt := err.(*gojq.HaltError); halt {
				return nil
			}
			return &JQError{Expr: opts.JQ, Err: err}
		}
		if s, isStr := x.(string); isStr {
			if _, err := fmt.Fprintln(w, s); err != nil {
				return err
			}
			continue
		}
		if err := writeJSON(w, x, opts.Pretty, opts.Painter); err != nil {
			return err
		}
	}
}

// JQError reports a bad --jq expression.
type JQError struct {
	Expr string
	Err  error
}

func (e *JQError) Error() string { return fmt.Sprintf("--jq %q: %v", e.Expr, e.Err) }

// normalizeNumbers converts integral float64 values to int so jq prints 3, not 3.0e0.
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
			return int(x)
		}
		return x
	case []any:
		for i := range x {
			x[i] = normalizeNumbers(x[i])
		}
		return x
	case map[string]any:
		for k := range x {
			x[k] = normalizeNumbers(x[k])
		}
		return x
	}
	return v
}

// ListItems extracts the list part of a response: the array itself, or the
// "items"/"infos" array of an object.
func ListItems(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case map[string]any:
		for _, k := range []string{"items", "infos", "results"} {
			if list, ok := x[k].([]any); ok {
				return list, true
			}
		}
	}
	return nil, false
}

func writeJSONL(w io.Writer, v any) error {
	if list, ok := ListItems(v); ok {
		for _, item := range list {
			if err := WriteJSON(w, item, false); err != nil {
				return err
			}
		}
		return nil
	}
	return WriteJSON(w, v, false)
}

func columns(rows []any, preferred []string) []string {
	first, _ := rows[0].(map[string]any)
	if len(preferred) > 0 {
		var cols []string
		for _, c := range preferred {
			if _, ok := first[c]; ok {
				cols = append(cols, c)
			}
		}
		if len(cols) > 0 {
			return cols
		}
	}
	seen := map[string]bool{}
	var cols []string
	for _, r := range rows {
		m, _ := r.(map[string]any)
		for k, v := range m {
			switch v.(type) {
			case map[string]any, []any:
				continue
			}
			if seen[k] {
				continue
			}
			if !columnHasValue(rows, k) {
				continue
			}
			seen[k] = true
			cols = append(cols, k)
		}
	}
	sort.SliceStable(cols, func(i, j int) bool {
		pi, pj := columnPriority(cols[i]), columnPriority(cols[j])
		if pi != pj {
			return pi < pj
		}
		return cols[i] < cols[j]
	})
	if len(cols) > 8 {
		cols = cols[:8]
	}
	return cols
}

// preferredColumns are shown first (in this order) when a table picks its
// columns automatically, so nested lists such as contacts show names and
// phones rather than the alphabetically first fields.
var preferredColumns = []string{
	"contactType", "type", "name", "domainName", "handle", "firstName", "lastName", "companyName",
	"eMail", "phoneCountryCode", "phone", "country", "city", "status", "content", "ipAddress", "ipVersion",
	"keyTag", "algorithm", "digestType", "digest", "price", "currency", "period",
}

func columnPriority(k string) int {
	for i, c := range preferredColumns {
		if c == k {
			return i
		}
	}
	return len(preferredColumns)
}

func columnHasValue(rows []any, k string) bool {
	for _, r := range rows {
		m, _ := r.(map[string]any)
		switch v := m[k].(type) {
		case nil:
		case string:
			if v != "" {
				return true
			}
		default:
			return true
		}
	}
	return false
}

var dateTime = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})?$`)

// summarize picks the most telling value of an object for a table cell.
func summarize(m map[string]any) string {
	for _, k := range []string{"content", "name", "servicePlan", "domainName", "handle", "ipAddress", "value"} {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// humanize turns camelCase keys into words: expirationDate -> expiration date.
func humanize(k string) string {
	if k == "eMail" {
		return "e-mail"
	}
	var sb strings.Builder
	for i, r := range k {
		if r >= 'A' && r <= 'Z' && i > 0 {
			sb.WriteByte(' ')
			sb.WriteRune(r + ('a' - 'A'))
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
