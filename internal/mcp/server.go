// Package mcp implements a Model Context Protocol server (stdio transport)
// that exposes every DomainNameAPI operation as a tool. Tools, their input
// schemas and safety annotations are generated from the operation registry,
// so the MCP surface always matches the CLI.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/openapi"
	"github.com/domainreseller/cli/internal/ops"
)

// SupportedVersions are the MCP protocol revisions this server speaks,
// newest first.
var SupportedVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Options configure the server.
type Options struct {
	// Client returns the API client. It is called lazily, so the server can
	// start (and list tools) before credentials are configured.
	Client func() (*dna.Client, error)
	// Environment and BaseURL are shown to the model so it knows where it acts.
	Environment string
	BaseURL     string
	Version     string
	// ReadOnly hides every tool that changes something.
	ReadOnly bool
	// NoBillable hides billable tools (register, renew, transfer, ...).
	NoBillable bool
	// Groups limits tools to these top-level groups (empty = all).
	Groups []string
	// Log receives diagnostics (stderr). May be nil.
	Log io.Writer
}

// Server is an MCP server.
type Server struct {
	opts   Options
	tools  []*ops.Operation
	byName map[string]*ops.Operation

	mu       sync.Mutex // guards out
	out      *json.Encoder
	inflight sync.Map // request id -> context.CancelFunc
	wg       sync.WaitGroup
}

// New creates a server.
func New(opts Options) *Server {
	s := &Server{opts: opts, byName: map[string]*ops.Operation{}}
	for _, op := range ops.All() {
		if opts.ReadOnly && op.Safety.Mutates() {
			continue
		}
		if opts.NoBillable && op.Safety == ops.Billable {
			continue
		}
		if len(opts.Groups) > 0 && !containsFold(opts.Groups, op.CommandPath()[0]) {
			continue
		}
		s.tools = append(s.tools, op)
		s.byName[op.ToolName()] = op
	}
	return s
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), s) {
			return true
		}
	}
	return false
}

// Tool is an MCP tool definition.
type Tool struct {
	Name        string              `json:"name"`
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description"`
	InputSchema *openapi.JSONSchema `json:"inputSchema"`
	Annotations map[string]any      `json:"annotations,omitempty"`
}

const contextTool = "dna_context"

// Tools returns the tool definitions the server advertises.
func (s *Server) Tools() []Tool {
	tools := []Tool{{
		Name:        contextTool,
		Title:       "Show the DomainNameAPI environment this server acts on",
		Description: "Returns the environment (production or ote), base URL, whether read-only mode is on and which tool groups are enabled. Call it first if unsure whether actions are real (production) or test (ote). Makes no API call.",
		InputSchema: &openapi.JSONSchema{Type: "object"},
		Annotations: map[string]any{"title": "Environment info", "readOnlyHint": true, "openWorldHint": false},
	}}
	for _, op := range s.tools {
		tools = append(tools, s.toolFor(op))
	}
	return tools
}

func (s *Server) toolFor(op *ops.Operation) Tool {
	schema := op.InputSchema()
	var desc strings.Builder
	desc.WriteString(op.Summary + ".")
	if op.Description != "" {
		desc.WriteString(" " + ops.Neutral(op.Description))
	}
	switch op.Safety {
	case ops.Billable:
		desc.WriteString("\n\nBILLABLE: charges the reseller balance. Only call after the user explicitly approved this purchase; set confirm=true. Without confirm the call is not executed and returns a preview of the request.")
	case ops.Destructive:
		desc.WriteString("\n\nDESTRUCTIVE: deletes data or is hard to undo. Only call after the user explicitly approved it; set confirm=true. Without confirm the call returns a preview.")
	case ops.Write:
		desc.WriteString("\n\nChanges data (free).")
	}
	if op.Safety.NeedsConfirmation() {
		schema.Properties = append(schema.Properties, openapi.JSONProp{Name: "confirm", Schema: &openapi.JSONSchema{
			Type: "boolean", Description: "Must be true to execute. Set it only after the user approved this exact operation.",
		}})
	}
	if op.Paged() {
		schema.Properties = append(schema.Properties, openapi.JSONProp{Name: "all", Schema: &openapi.JSONSchema{
			Type: "boolean", Description: "Fetch every page and return all items (ignores skipCount).",
		}})
	}
	ann := map[string]any{
		"title":           op.Summary,
		"readOnlyHint":    !op.Safety.Mutates(),
		"destructiveHint": op.Safety.NeedsConfirmation(),
		"idempotentHint":  !op.Safety.Mutates() || op.Idempotent,
		"openWorldHint":   true,
	}
	if !op.Safety.Mutates() {
		delete(ann, "destructiveHint")
	}
	return Tool{Name: op.ToolName(), Title: op.Summary, Description: desc.String(), InputSchema: schema, Annotations: ann}
}

func (s *Server) instructions() string {
	env := s.opts.Environment
	if env == "" {
		env = "production"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "DomainNameAPI reseller tools. Environment: %s (%s).", env, s.opts.BaseURL)
	if env == string(dna.Production) {
		b.WriteString(" This is the LIVE environment: registrations, renewals and transfers are real and charge the balance.")
	} else {
		b.WriteString(" This is the OTE test environment: nothing affects live domains or the live balance.")
	}
	if s.opts.ReadOnly {
		b.WriteString(" Read-only mode: only tools without side effects are available.")
	}
	b.WriteString(" Guidelines: check availability and price with domain_search before domain_register; check domain_renew_check before domain_renew and domain_transfer_check before domain_transfer." +
		" Billable and destructive tools require confirm=true, which you may only set after the user explicitly approved that exact action; calling without it returns a preview you can show the user." +
		" Numeric codes (status, type) are explained in each tool's schema; enum inputs accept names. List tools are paged (skipCount/maxResultCount) or take all=true." +
		" Errors come back as JSON with kind, code, message and hint.")
	return b.String()
}

// ---------------------------------------------------------------- JSON-RPC

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Serve reads newline-delimited JSON-RPC messages from in and writes
// responses to out until in is closed or ctx is cancelled.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = json.NewEncoder(out)
	s.out.SetEscapeHTML(false)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	lines := make(chan []byte)
	errc := make(chan error, 1)
	go func() {
		for sc.Scan() {
			line := append([]byte(nil), sc.Bytes()...)
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		errc <- sc.Err()
		close(lines)
	}()
	for {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			return ctx.Err()
		case line, ok := <-lines:
			if !ok {
				s.wg.Wait()
				return <-errc
			}
			s.handleLine(ctx, line)
		}
	}
}

func (s *Server) logf(format string, args ...any) {
	if s.opts.Log != nil {
		fmt.Fprintf(s.opts.Log, "dna mcp: "+format+"\n", args...)
	}
}

func (s *Server) send(resp rpcResponse) {
	resp.JSONRPC = "2.0"
	if resp.ID == nil {
		resp.ID = json.RawMessage("null")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.out.Encode(resp); err != nil {
		s.logf("write: %v", err)
	}
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return
	}
	if strings.HasPrefix(trimmed, "[") { // batches (2025-03-26) are answered one by one
		var batch []json.RawMessage
		if err := json.Unmarshal(line, &batch); err != nil {
			s.send(rpcResponse{Error: &rpcError{Code: codeParse, Message: "parse error"}})
			return
		}
		for _, m := range batch {
			s.handleLine(ctx, m)
		}
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		s.send(rpcResponse{Error: &rpcError{Code: codeParse, Message: "parse error: " + err.Error()}})
		return
	}
	if req.Method == "" {
		return // a response from the client (we send no requests); ignore
	}
	isNotification := len(req.ID) == 0
	switch req.Method {
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(req.Params, &p) == nil {
			if cancel, ok := s.inflight.Load(string(p.RequestID)); ok {
				cancel.(context.CancelFunc)()
			}
		}
		return
	case "tools/call":
		if isNotification {
			return
		}
		cctx, cancel := context.WithCancel(ctx)
		s.inflight.Store(string(req.ID), cancel)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer cancel()
			defer s.inflight.Delete(string(req.ID))
			result, rerr := s.callTool(cctx, req.Params)
			s.send(rpcResponse{ID: req.ID, Result: result, Error: rerr})
		}()
		return
	}
	if isNotification {
		return
	}
	result, rerr := s.dispatch(req)
	s.send(rpcResponse{ID: req.ID, Result: result, Error: rerr})
}

func (s *Server) dispatch(req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
			ClientInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"clientInfo"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := SupportedVersions[0]
		for _, v := range SupportedVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		s.logf("client %s %s connected (protocol %s), %d tools", p.ClientInfo.Name, p.ClientInfo.Version, version, len(s.tools)+1)
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "dna", "title": "DomainNameAPI", "version": s.opts.Version},
			"instructions":    s.instructions(),
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.Tools()}, nil
	case "resources/list":
		return map[string]any{"resources": []any{}}, nil
	case "prompts/list":
		return map[string]any{"prompts": []any{}}, nil
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
}

// toolResult is the MCP CallToolResult.
type toolResult struct {
	Content           []map[string]any `json:"content"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	IsError           bool             `json:"isError"`
}

func textResult(v any, isError bool) *toolResult {
	b, _ := json.Marshal(v)
	r := &toolResult{Content: []map[string]any{{"type": "text", "text": string(b)}}, IsError: isError}
	if m, ok := v.(map[string]any); ok {
		r.StructuredContent = m
	}
	return r
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params: " + err.Error()}
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	if p.Name == contextTool {
		groups := s.opts.Groups
		if len(groups) == 0 {
			groups = []string{"all"}
		}
		return textResult(map[string]any{
			"environment": s.opts.Environment, "baseUrl": s.opts.BaseURL, "readOnly": s.opts.ReadOnly,
			"billableToolsEnabled": !s.opts.NoBillable && !s.opts.ReadOnly, "groups": groups, "tools": len(s.tools),
			"serverVersion": s.opts.Version,
		}, false), nil
	}
	op, ok := s.byName[p.Name]
	if !ok {
		if known, exists := ops.Lookup(p.Name); exists && known.ToolName() == p.Name {
			return textResult(errPayload("disabled", "tool "+p.Name+" is disabled on this server (read-only or filtered)", ""), true), nil
		}
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool: " + p.Name}
	}

	confirm, _ := p.Arguments["confirm"].(bool)
	all, _ := p.Arguments["all"].(bool)
	delete(p.Arguments, "confirm")
	delete(p.Arguments, "all")

	req, err := op.BuildRequest(ops.Input(p.Arguments))
	if err != nil {
		var ie *ops.InputError
		if errors.As(err, &ie) {
			out := errPayload("invalid_input", ie.Error(), "Fix the listed fields; the tool's inputSchema lists every field and its allowed values.")
			out["error"].(map[string]any)["fields"] = ie.Errors
			return textResult(out, true), nil
		}
		return textResult(errPayload("invalid_input", err.Error(), ""), true), nil
	}
	if op.Safety.NeedsConfirmation() && !confirm {
		preview := map[string]any{"method": req.Method, "path": req.Path}
		if len(req.Query) > 0 {
			preview["query"] = req.Query
		}
		if req.Body != nil {
			preview["body"] = req.Body
		}
		out := errPayload("confirmation_required",
			fmt.Sprintf("%s is %s and was NOT executed. Show the user what will happen (environment %s) and ask for approval; then call again with the same arguments plus confirm=true.", op.ToolName(), op.Safety, s.opts.Environment),
			"")
		out["preview"] = preview
		return textResult(out, true), nil
	}
	client, err := s.opts.Client()
	if err != nil {
		return textResult(errPayload("auth", err.Error(), "Configure credentials: `dna auth login` or DNA_RESELLER_ID / DNA_API_KEY in the MCP server's environment."), true), nil
	}
	if !client.HasCredentials() {
		return textResult(errPayload("auth", dna.ErrNoCredentials.Error(), "Configure credentials: `dna auth login` or DNA_RESELLER_ID / DNA_API_KEY in the MCP server's environment."), true), nil
	}

	var data json.RawMessage
	if all && op.Paged() {
		data, err = fetchAll(ctx, client, req)
	} else {
		_, err = client.Do(ctx, req, &data)
	}
	if err != nil {
		return textResult(apiErrPayload(err), true), nil
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return textResult(map[string]any{"success": true}, false), nil
	}
	var v any
	if json.Unmarshal(data, &v) != nil {
		return &toolResult{Content: []map[string]any{{"type": "text", "text": string(data)}}}, nil
	}
	if list, isList := v.([]any); isList {
		r := textResult(map[string]any{"items": list}, false)
		r.Content[0]["text"] = string(data)
		return r, nil
	}
	r := textResult(v, false)
	r.Content[0]["text"] = string(data)
	return r, nil
}

func errPayload(kind, msg, hint string) map[string]any {
	e := map[string]any{"kind": kind, "message": msg}
	if hint != "" {
		e["hint"] = hint
	}
	return map[string]any{"error": e}
}

func apiErrPayload(err error) map[string]any {
	var ae *dna.APIError
	if errors.As(err, &ae) {
		e := map[string]any{"kind": string(ae.Kind()), "code": ae.Code, "message": ae.Message, "status": ae.StatusCode, "retryable": ae.Retryable()}
		if ae.Details != "" && ae.Details != ae.Message {
			e["details"] = ae.Details
		}
		if len(ae.ValidationErrors) > 0 {
			e["validationErrors"] = ae.ValidationErrors
		}
		if ae.CorrelationID != "" {
			e["correlationId"] = ae.CorrelationID
		}
		if h := ae.Hint(); h != "" {
			e["hint"] = h
		}
		return map[string]any{"error": e}
	}
	var ne *dna.NetworkError
	if errors.As(err, &ne) {
		kind := "network"
		if ne.Timeout() {
			kind = "timeout"
		}
		return errPayload(kind, err.Error(), "For billable operations, check the outcome (e.g. domain_info) before retrying.")
	}
	return errPayload("error", err.Error(), "")
}

func fetchAll(ctx context.Context, client *dna.Client, req *dna.Request) (json.RawMessage, error) {
	q := map[string][]string{}
	for k, v := range req.Query {
		q[k] = v
	}
	size := 100
	skip := 0
	var items []json.RawMessage
	var total int64
	for page := 0; page < 1000; page++ {
		q["SkipCount"] = []string{fmt.Sprint(skip)}
		q["MaxResultCount"] = []string{fmt.Sprint(size)}
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
