package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/domainreseller/cli/internal/config"
	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ops"
	"github.com/domainreseller/cli/internal/output"
)

// subgroupHelp describes intermediate command groups such as `domain host`.
var subgroupHelp = map[string]string{
	"domain contacts":    "Contacts of a domain",
	"domain documents":   "Registry documents of a domain (mostly .tr)",
	"domain forward":     "URL forwarding of a domain",
	"domain nameservers": "Nameservers of a domain",
	"domain host":        "Child nameservers (glue records) such as ns1.example.com",
}

// argName renders a positional argument for usage lines.
func argName(key string) string {
	switch key {
	case "domainName":
		return "domain"
	case "handleCode":
		return "handle"
	case "nameServers":
		return "nameserver"
	case "hostName":
		return "host"
	case "privacyStatus":
		return "on|off"
	case "redirectAddress":
		return "url"
	}
	return flagName(key)
}

func flagName(key string) string {
	var sb strings.Builder
	for i, r := range key {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				sb.WriteByte('-')
			}
			sb.WriteRune(r + ('a' - 'A'))
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func useLine(op *ops.Operation) string {
	path := op.CommandPath()
	parts := []string{path[len(path)-1]}
	for _, a := range op.Args {
		if strings.HasSuffix(a, "...") {
			parts = append(parts, "<"+argName(strings.TrimSuffix(a, "..."))+">...")
		} else {
			parts = append(parts, "<"+argName(a)+">")
		}
	}
	return strings.Join(parts, " ")
}

func safetyLine(s ops.Safety) string {
	switch s {
	case ops.Read:
		return "read-only (no side effects)"
	case ops.Write:
		return "changes data (free)"
	case ops.Destructive:
		return "DESTRUCTIVE - deletes data or is hard to undo; asks for confirmation (--yes to skip)"
	case ops.Billable:
		return "BILLABLE - charges your balance; asks for confirmation (--yes to skip)"
	}
	return string(s)
}

func longHelp(op *ops.Operation) string {
	var sb strings.Builder
	if op.Description != "" {
		sb.WriteString(op.Summary + ".\n\n" + wrap(op.Description, 100) + "\n\n")
	} else {
		sb.WriteString(op.Summary + ".\n\n")
	}
	fmt.Fprintf(&sb, "Safety:     %s\n", safetyLine(op.Safety))
	fmt.Fprintf(&sb, "API:        %s %s\n", op.Method, op.Path)
	if t := op.BodyType(); t != "" {
		fmt.Fprintf(&sb, "Body:       %s\n", t)
	}
	if t := op.ResultType(); t != "" {
		fmt.Fprintf(&sb, "Returns:    %s\n", t)
	} else {
		sb.WriteString("Returns:    nothing (prints {\"success\":true})\n")
	}
	fmt.Fprintf(&sb, "Schema:     dna schema %s   (MCP tool: %s)\n", op.ID, op.ToolName())
	sb.WriteString("\nEvery input can also be given as JSON with --body (inline, @file or - for stdin); flags override it.")
	return sb.String()
}

func examples(op *ops.Operation) string {
	var lines []string
	for _, e := range op.Examples {
		if e.Description != "" {
			lines = append(lines, "  # "+e.Description)
		}
		lines = append(lines, "  "+e.Command)
	}
	return strings.Join(lines, "\n")
}

func wrap(s string, width int) string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, w := range words {
			if line != "" && len(line)+1+len(w) > width {
				out = append(out, line)
				line = w
				continue
			}
			if line != "" {
				line += " "
			}
			line += w
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func fieldHelp(op *ops.Operation, f *ops.InputField) string {
	help := f.Help
	if help == "" || strings.HasPrefix(help, "(") {
		help = strings.TrimSpace(humanizeKey(f.Path[len(f.Path)-1]) + " " + help)
	}
	if f.Enum != nil {
		if len(f.Enum.Values) <= 14 {
			help += " [" + f.Enum.Summary() + "]"
		} else {
			help += " (see 'dna enums " + f.Enum.Name + "')"
		}
	}
	switch f.Kind {
	case ops.KindJSON:
		help += " (JSON, @file or -)"
	case ops.KindMap:
		help += " (KEY=VALUE, repeatable)"
	case ops.KindStrings:
		if !strings.Contains(help, "repeat") {
			help += " (repeatable)"
		}
	}
	if f.File {
		help += " (@file reads a file)"
	}
	var alt []string
	for i, a := range op.Args {
		if strings.TrimSuffix(a, "...") == f.Key {
			alt = append(alt, fmt.Sprintf("argument %d <%s>", i+1, argName(f.Key)))
		}
	}
	for _, c := range op.Converters {
		if c.Target == f.Key {
			alt = append(alt, "--"+c.Flag)
		}
	}
	switch {
	case f.Required && len(alt) > 0:
		help += " (required; or " + strings.Join(alt, " / ") + ")"
	case f.Required:
		help += " (required)"
	case len(alt) > 0:
		help += " (or " + strings.Join(alt, " / ") + ")"
	}
	return help
}

func humanizeKey(k string) string {
	if k == "eMail" {
		return "E-mail"
	}
	s := strings.ReplaceAll(flagName(k), "-", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type boundField struct {
	field *ops.InputField
	flag  *pflag.Flag
}

// newOpCommand builds the cobra command for one API operation.
func (a *App) newOpCommand(op *ops.Operation) *cobra.Command {
	cmd := &cobra.Command{
		Use:         useLine(op),
		Short:       op.Summary,
		Long:        longHelp(op),
		Example:     examples(op),
		Aliases:     op.Aliases,
		Annotations: map[string]string{"operation": op.ID, "safety": string(op.Safety)},
	}
	variadic := len(op.Args) > 0 && strings.HasSuffix(op.Args[len(op.Args)-1], "...")
	if variadic {
		cmd.Args = cobra.ArbitraryArgs
	} else {
		cmd.Args = cobra.MaximumNArgs(len(op.Args))
	}

	fs := cmd.Flags()
	var bound []boundField
	for _, f := range op.Inputs() {
		if f.Hidden {
			continue
		}
		help := fieldHelp(op, f)
		switch f.Kind {
		case ops.KindInt:
			fs.IntP(f.Flag, f.Short, 0, help)
		case ops.KindNumber:
			fs.Float64P(f.Flag, f.Short, 0, help)
		case ops.KindBool:
			fs.BoolP(f.Flag, f.Short, false, help)
		case ops.KindStrings, ops.KindMap:
			fs.StringArrayP(f.Flag, f.Short, nil, help)
		default:
			fs.StringP(f.Flag, f.Short, "", help)
		}
		bound = append(bound, boundField{f, fs.Lookup(f.Flag)})
	}
	for _, c := range op.Converters {
		fs.StringArray(c.Flag, nil, c.Help)
	}
	fs.StringP("body", "d", "", "Full input as JSON: inline, @file, or - to read stdin")
	if op.Paged() {
		fs.Bool("all", false, "Fetch every page and return all items")
	}
	fs.SortFlags = false

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		in, err := a.collectInput(cmd, op, bound, args)
		if err != nil {
			return err
		}
		all, _ := fs.GetBool("all")
		return a.runOperation(cmd.Context(), op, in, all)
	}
	return cmd
}

// collectInput merges --body, flags, converters and positional arguments.
func (a *App) collectInput(cmd *cobra.Command, op *ops.Operation, bound []boundField, args []string) (ops.Input, error) {
	fs := cmd.Flags()
	in := ops.Input{}
	if body, _ := fs.GetString("body"); body != "" {
		if t := strings.TrimSpace(body); t != "-" && !strings.HasPrefix(t, "@") && !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
			return nil, usagef("-d/--body expects a JSON object, @file or - (stdin), got %q.\n"+
				"Single-dash words are read as shorthands (-domainName becomes -d omainName). Flags use two dashes, e.g. --domain-name.\n"+
				"Usage: %s", body, cmd.UseLine())
		}
		v, err := a.readJSONArg("--body", body)
		if err != nil {
			return nil, err
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, usagef("--body must be a JSON object with the operation's inputs (see `dna schema %s`)", op.ID)
		}
		for k, val := range m {
			in[k] = val
		}
	}
	for _, b := range bound {
		if !b.flag.Changed {
			continue
		}
		f := b.field
		var val any
		switch f.Kind {
		case ops.KindInt:
			val, _ = fs.GetInt(f.Flag)
		case ops.KindNumber:
			val, _ = fs.GetFloat64(f.Flag)
		case ops.KindBool:
			val, _ = fs.GetBool(f.Flag)
		case ops.KindStrings:
			vals, _ := fs.GetStringArray(f.Flag)
			if f.Split {
				vals = splitCommas(vals)
			}
			val = vals
		case ops.KindMap:
			vals, _ := fs.GetStringArray(f.Flag)
			m := map[string]string{}
			for _, kv := range vals {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return nil, usagef("--%s %q: expected KEY=VALUE", f.Flag, kv)
				}
				m[strings.TrimSpace(k)] = v
			}
			val = m
		case ops.KindJSON:
			s, _ := fs.GetString(f.Flag)
			v, err := a.readJSONArg("--"+f.Flag, s)
			if err != nil {
				return nil, err
			}
			val = v
		default:
			s, _ := fs.GetString(f.Flag)
			if f.File && strings.HasPrefix(s, "@") {
				content, err := ops.ReadValue(s)
				if err != nil {
					return nil, usagef("--%s: %v", f.Flag, err)
				}
				s = content
			}
			val = s
		}
		setPath(in, f.Path, val)
	}
	for _, c := range op.Converters {
		vals, _ := fs.GetStringArray(c.Flag)
		if len(vals) == 0 {
			continue
		}
		v, err := c.Build(vals)
		if err != nil {
			return nil, usageErr(err)
		}
		if fl := fs.Lookup(flagName(c.Target)); fl != nil && fl.Changed {
			return nil, usagef("use either --%s or --%s, not both", c.Flag, fl.Name)
		}
		in[c.Target] = v
	}
	for i, arg := range op.Args {
		key := strings.TrimSuffix(arg, "...")
		var val any
		if strings.HasSuffix(arg, "...") {
			if i >= len(args) {
				break
			}
			rest := splitCommas(args[i:])
			val = rest
		} else {
			if i >= len(args) {
				break
			}
			val = args[i]
		}
		if cur, ok := in[key]; ok && !isZero(cur) && fmt.Sprint(cur) != fmt.Sprint(val) {
			if s, isList := cur.([]string); isList {
				val = append(s, val.([]string)...)
			} else {
				return nil, usagef("%s given both as argument (%v) and flag (%v)", argName(key), val, cur)
			}
		}
		in[key] = val
	}
	return in, nil
}

func isZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []string:
		return len(x) == 0
	}
	return false
}

func splitCommas(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func setPath(in ops.Input, path []string, v any) {
	if len(path) == 1 {
		in[path[0]] = v
		return
	}
	m, ok := in[path[0]].(map[string]any)
	if !ok {
		m = map[string]any{}
	}
	m[path[1]] = v
	in[path[0]] = m
}

// readJSONArg parses a JSON flag value: inline JSON, @file or - (stdin).
func (a *App) readJSONArg(flag, s string) (any, error) {
	var raw []byte
	switch {
	case s == "-":
		b, err := io.ReadAll(a.In)
		if err != nil {
			return nil, usagef("%s: reading stdin: %v", flag, err)
		}
		raw = b
	case strings.HasPrefix(s, "@"):
		content, err := ops.ReadValue(s)
		if err != nil {
			return nil, usagef("%s: %v", flag, err)
		}
		raw = []byte(content)
	default:
		raw = []byte(s)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, usagef("%s: invalid JSON: %v", flag, err)
	}
	return v, nil
}

// runOperation validates, guards, sends and renders one operation.
func (a *App) runOperation(ctx context.Context, op *ops.Operation, in ops.Input, all bool) error {
	req, err := op.BuildRequest(in)
	if err != nil {
		return explainPositional(op, err)
	}
	res, err := a.resolved()
	if err != nil {
		return err
	}
	if res.ReadOnly && op.Safety.Mutates() {
		return &PolicyError{Operation: op.ID, Source: res.ReadOnlySource}
	}
	client, err := a.client()
	if err != nil {
		return err
	}
	if a.flags.dryRun {
		return a.printDryRun(ctx, client, op, req, res)
	}
	if op.Safety.NeedsConfirmation() && !a.flags.yes {
		if err := a.confirm(op, req, res); err != nil {
			return err
		}
	}
	if all && op.Paged() {
		data, err := a.fetchAll(ctx, client, req)
		if err != nil {
			return err
		}
		return a.render(data, output.Options{Columns: op.Columns, Enums: op.ResultEnums(), RowHook: op.TableRow})
	}
	var raw json.RawMessage
	resp, err := client.Do(ctx, req, &raw)
	if err != nil {
		return err
	}
	if resp != nil && resp.Quota != nil && resp.Quota.Delay >= 1 && !a.machineOutput() {
		a.warn(fmt.Sprintf("the API slowed this request down by %.1fs (quota %s %.0f/%.0f); it recovers within %.0fs", resp.Quota.Delay, resp.Quota.Scope, resp.Quota.Used, resp.Quota.Limit, resp.Quota.Reset))
	}
	return a.render(raw, output.Options{Columns: op.Columns, Enums: op.ResultEnums(), RowHook: op.TableRow})
}

const pageSize = 100

func (a *App) fetchAll(ctx context.Context, client *dna.Client, req *dna.Request) ([]byte, error) {
	size := pageSize
	if s := req.Query.Get("MaxResultCount"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			size = n
		}
	}
	skip := 0
	if s := req.Query.Get("SkipCount"); s != "" {
		skip, _ = strconv.Atoi(s)
	}
	var items []json.RawMessage
	var total int64
	for page := 0; page < 1000; page++ {
		q := url.Values{}
		for k, v := range req.Query {
			q[k] = v
		}
		q.Set("SkipCount", strconv.Itoa(skip))
		q.Set("MaxResultCount", strconv.Itoa(size))
		var pr struct {
			Items      []json.RawMessage `json:"items"`
			TotalCount int64             `json:"totalCount"`
		}
		if _, err := client.Do(ctx, &dna.Request{Method: req.Method, Path: req.Path, Query: q}, &pr); err != nil {
			return nil, err
		}
		items = append(items, pr.Items...)
		total = pr.TotalCount
		skip += len(pr.Items)
		if len(pr.Items) == 0 || int64(skip) >= total {
			break
		}
	}
	if items == nil {
		items = []json.RawMessage{}
	}
	return json.Marshal(map[string]any{"items": items, "totalCount": total})
}

func (a *App) printDryRun(ctx context.Context, client *dna.Client, op *ops.Operation, req *dna.Request, res *config.Resolved) error {
	hreq, body, err := client.NewHTTPRequest(ctx, req)
	if err != nil {
		return err
	}
	headers := map[string]string{}
	for k, v := range hreq.Header {
		val := strings.Join(v, ",")
		switch strings.ToLower(k) {
		case "x-api-key":
			val = config.MaskKey(val)
		}
		headers[k] = val
	}
	out := map[string]any{
		"dryRun":      true,
		"operation":   op.ID,
		"safety":      op.Safety,
		"environment": res.Environment,
		"request": map[string]any{
			"method":  hreq.Method,
			"url":     hreq.URL.String(),
			"headers": headers,
		},
	}
	if len(body) > 0 {
		var v any
		_ = json.Unmarshal(body, &v)
		out["request"].(map[string]any)["body"] = v
	}
	if !res.HasCredentials() {
		out["warning"] = "no credentials configured; the real request would fail with exit code 3"
	}
	return a.printJSON(out)
}

// confirm asks the user to approve a billable or destructive operation.
func (a *App) confirm(op *ops.Operation, req *dna.Request, res *config.Resolved) error {
	if !a.interactive() {
		return &ConfirmationError{Operation: op.ID, Safety: op.Safety}
	}
	env := strings.ToUpper(string(res.Environment))
	label := "BILLABLE"
	if op.Safety == ops.Destructive {
		label = "DESTRUCTIVE"
	}
	fmt.Fprintf(a.Err, "%s %s on %s (%s)\n", a.color("33;1", label+":"), op.Summary, a.color("1", env), res.BaseURL)
	desc := req.Method + " " + req.Path
	if len(req.Query) > 0 {
		desc += "?" + req.Query.Encode()
	}
	fmt.Fprintf(a.Err, "  %s\n", desc)
	if req.Body != nil {
		b, _ := json.Marshal(req.Body)
		if len(b) > 400 {
			b = append(b[:400], []byte("…")...)
		}
		fmt.Fprintf(a.Err, "  %s\n", b)
	}
	fmt.Fprint(a.Err, "Continue? [y/N] ")
	line, _ := bufio.NewReader(a.In).ReadString('\n')
	if ok, valid := ops.ParseBool(strings.TrimSpace(line)); !valid || !ok {
		return &ConfirmationError{Operation: op.ID, Safety: op.Safety, Declined: true}
	}
	return nil
}

// explainPositional rewrites "is required" input errors for positional
// arguments so they show how to pass them: `dna domain search <domain>`.
func explainPositional(op *ops.Operation, err error) error {
	ie, ok := err.(*ops.InputError)
	if !ok {
		return err
	}
	flags := map[string]string{}
	for _, f := range op.Inputs() {
		flags[f.Key] = f.Flag
	}
	cmdLine := "dna " + strings.Join(op.CommandPath()[:len(op.CommandPath())-1], " ") + " " + useLine(op)
	out := &ops.InputError{Operation: ie.Operation}
	for _, fe := range ie.Errors {
		for _, arg := range op.Args {
			key := strings.TrimSuffix(arg, "...")
			if fe.Field == key && fe.Message == "is required" {
				fe.Message = fmt.Sprintf("is required: %s (or --%s)", cmdLine, flags[key])
			}
		}
		out.Errors = append(out.Errors, fe)
	}
	return out
}
