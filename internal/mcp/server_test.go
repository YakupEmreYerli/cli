package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/domainreseller/cli/internal/dna"
)

func serve(t *testing.T, s *Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var msgs []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("bad JSON %q: %v", l, err)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func noClient() (*dna.Client, error) { return dna.NewClient("", "") }

func TestReadOnlyHidesMutatingTools(t *testing.T) {
	s := New(Options{Client: noClient, ReadOnly: true})
	for _, tool := range s.Tools() {
		if ro, _ := tool.Annotations["readOnlyHint"].(bool); !ro {
			t.Errorf("read-only server exposes %s", tool.Name)
		}
	}
	msgs := serve(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"domain_register","arguments":{}}}`)
	res := msgs[0]["result"].(map[string]any)
	if res["isError"] != true || !strings.Contains(res["content"].([]any)[0].(map[string]any)["text"].(string), "disabled") {
		t.Errorf("disabled tool result = %v", res)
	}
}

func TestNoBillableAndGroups(t *testing.T) {
	s := New(Options{Client: noClient, NoBillable: true, Groups: []string{"dns"}})
	for _, tool := range s.Tools()[1:] {
		if !strings.HasPrefix(tool.Name, "dns_") {
			t.Errorf("group filter leaked %s", tool.Name)
		}
	}
	s = New(Options{Client: noClient, NoBillable: true})
	for _, tool := range s.Tools() {
		if tool.Name == "domain_register" || tool.Name == "domain_renew" {
			t.Errorf("billable tool %s exposed", tool.Name)
		}
	}
}

func TestProtocolBasics(t *testing.T) {
	s := New(Options{Client: noClient, Environment: "ote", BaseURL: dna.OTEURL, Version: "1.0"})
	msgs := serve(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"dna_context","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"domain_info","arguments":{"domainNme":"x"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"domain_info","arguments":{"domainName":"example.com"}}}`,
		`not json`,
	)
	byID := map[float64]map[string]any{}
	for _, m := range msgs {
		if id, ok := m["id"].(float64); ok {
			byID[id] = m
		}
	}
	init := byID[1]["result"].(map[string]any)
	if init["protocolVersion"] != SupportedVersions[0] || !strings.Contains(init["instructions"].(string), "OTE") {
		t.Errorf("initialize = %v", init)
	}
	if byID[3]["error"].(map[string]any)["code"].(float64) != codeMethodNotFound {
		t.Errorf("unknown method = %v", byID[3])
	}
	if byID[4]["error"] == nil {
		t.Errorf("unknown tool should be a protocol error: %v", byID[4])
	}
	ctx := byID[5]["result"].(map[string]any)["structuredContent"].(map[string]any)
	if ctx["environment"] != "ote" {
		t.Errorf("dna_context = %v", ctx)
	}
	bad := byID[6]["result"].(map[string]any)
	if bad["isError"] != true || !strings.Contains(bad["content"].([]any)[0].(map[string]any)["text"].(string), "invalid_input") {
		t.Errorf("invalid input = %v", bad)
	}
	nocred := byID[7]["result"].(map[string]any)
	if nocred["isError"] != true || !strings.Contains(nocred["content"].([]any)[0].(map[string]any)["text"].(string), `"kind":"auth"`) {
		t.Errorf("missing credentials = %v", nocred)
	}
	parseErr := false
	for _, m := range msgs {
		if e, ok := m["error"].(map[string]any); ok && e["code"].(float64) == codeParse {
			parseErr = true
		}
	}
	if !parseErr {
		t.Error("expected a parse error response")
	}
}

func TestToolSchemasAreObjects(t *testing.T) {
	s := New(Options{Client: noClient})
	for _, tool := range s.Tools() {
		b, err := json.Marshal(tool.InputSchema)
		if err != nil || !strings.HasPrefix(string(b), `{"type":"object"`) {
			t.Errorf("%s: schema %s %v", tool.Name, b, err)
		}
		if len(tool.Name) > 64 || tool.Description == "" {
			t.Errorf("%s: bad name or description", tool.Name)
		}
	}
}
