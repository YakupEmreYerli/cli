// Package output renders API responses as JSON, JSON lines, tables or raw
// bytes, and applies --jq filters.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/itchyny/gojq"

	"github.com/domainreseller/cli/internal/dna"
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
}

// EmptySuccess is printed for successful calls that return no body.
var EmptySuccess = []byte(`{"success":true}`)

// Render writes data (a JSON document) to w.
func Render(w io.Writer, data []byte, opts Options) error {
	if len(bytes.TrimSpace(data)) == 0 {
		if opts.Format == Table && opts.JQ == "" {
			_, err := fmt.Fprintln(w, "OK")
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
		return writeTable(w, v, opts)
	}
	return WriteJSON(w, v, opts.Pretty)
}

// WriteJSON writes v as JSON followed by a newline.
func WriteJSON(w io.Writer, v any, pretty bool) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
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
		if err := WriteJSON(w, x, opts.Pretty); err != nil {
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

func writeTable(w io.Writer, v any, opts Options) error {
	if list, ok := ListItems(v); ok {
		if opts.RowHook != nil {
			for _, r := range list {
				if m, isObj := r.(map[string]any); isObj {
					opts.RowHook(m)
				}
			}
		}
		if err := writeRows(w, list, opts); err != nil {
			return err
		}
		if m, isObj := v.(map[string]any); isObj {
			if total, ok := m["totalCount"].(float64); ok && int(total) > len(list) {
				fmt.Fprintf(w, "\nShowing %d of %d. Use --all to fetch every page, or --skip-count/--max-result-count.\n", len(list), int(total))
			}
			for _, k := range []string{"operationMessage", "reason"} {
				if s, ok := m[k].(string); ok && s != "" {
					fmt.Fprintf(w, "%s: %s\n", k, s)
				}
			}
		}
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return writeObject(w, m, opts, "")
	}
	_, err := fmt.Fprintln(w, cell(v, nil))
	return err
}

func writeRows(w io.Writer, rows []any, opts Options) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No results.")
		return err
	}
	if _, isObj := rows[0].(map[string]any); !isObj {
		for _, r := range rows {
			fmt.Fprintln(w, cell(r, nil))
		}
		return nil
	}
	cols := columns(rows, opts.Columns)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = strings.ToUpper(humanize(c))
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		m, _ := r.(map[string]any)
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = truncate(cell(m[c], opts.Enums[c]), 60)
		}
		fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}
	return tw.Flush()
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

func writeObject(w io.Writer, m map[string]any, opts Options, indent string) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	var nested []string
	for _, k := range keys {
		switch x := m[k].(type) {
		case map[string]any:
			if len(x) > 0 {
				nested = append(nested, k)
				continue
			}
		case []any:
			if len(x) > 0 {
				if _, isObj := x[0].(map[string]any); isObj {
					nested = append(nested, k)
					continue
				}
			}
		}
		fmt.Fprintf(tw, "%s%s:\t%s\n", indent, humanize(k), cell(m[k], opts.Enums[k]))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, k := range nested {
		fmt.Fprintf(w, "\n%s%s:\n", indent, humanize(k))
		switch x := m[k].(type) {
		case map[string]any:
			if err := writeObject(w, x, Options{}, indent+"  "); err != nil {
				return err
			}
		case []any:
			var buf bytes.Buffer
			if err := writeRows(&buf, x, Options{}); err != nil {
				return err
			}
			for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
				fmt.Fprintf(w, "%s  %s\n", indent, line)
			}
		}
	}
	return nil
}

var dateTime = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})?$`)

func cell(v any, enum *dna.EnumInfo) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case string:
		if x == "" {
			return "-"
		}
		if m := dateTime.FindStringSubmatch(x); m != nil {
			if strings.HasPrefix(m[1], "0001-01-01") {
				return "-"
			}
			if m[2] == "00:00" {
				return m[1]
			}
			return m[1] + " " + m[2]
		}
		return strings.ReplaceAll(x, "\n", " ")
	case float64:
		s := strconv.FormatFloat(x, 'f', -1, 64)
		if enum != nil && x == math.Trunc(x) {
			if name := enum.NameOf(int(x)); name != "" {
				return name
			}
		}
		return s
	case bool:
		if x {
			return "yes"
		}
		return "no"
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				parts = append(parts, summarize(m))
				continue
			}
			parts = append(parts, cell(item, enum))
		}
		if len(parts) == 0 {
			return "-"
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		if len(x) == 0 {
			return "-"
		}
		b, _ := json.Marshal(x)
		return string(b)
	}
	return fmt.Sprint(v)
}

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

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
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
