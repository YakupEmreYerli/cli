package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	copyText = func(string) error { return nil } // tests never touch the real clipboard
}

func TestMCPConfigEveryClient(t *testing.T) {
	l := mcpLaunch{Bin: `C:\Program Files\dna\dna.exe`, Args: []string{"mcp", "serve", "--env", "ote"}}
	for _, a := range agents {
		text, err := mcpConfigText(a.ID, l)
		if err != nil || strings.TrimSpace(text) == "" {
			t.Errorf("%s: %v %q", a.ID, err, text)
			continue
		}
		if a.Kind == kindChat {
			continue
		}
		snippet, lang := l.snippet(a)
		switch lang {
		case "json":
			var v map[string]any
			if err := json.Unmarshal([]byte(snippet), &v); err != nil {
				t.Errorf("%s: invalid JSON: %v\n%s", a.ID, err, snippet)
			}
			if !strings.Contains(snippet, `"command"`) || !strings.Contains(snippet, `C:\\Program Files\\dna\\dna.exe`) {
				t.Errorf("%s: JSON misses the command:\n%s", a.ID, snippet)
			}
		case "toml":
			if !strings.Contains(snippet, "[mcp_servers.dna]") || !strings.Contains(snippet, `command = 'C:\Program Files\dna\dna.exe'`) {
				t.Errorf("%s: toml =\n%s", a.ID, snippet)
			}
		default:
			if !strings.Contains(snippet, "mcp") || !strings.Contains(snippet, "--env") {
				t.Errorf("%s: snippet misses the server command:\n%s", a.ID, snippet)
			}
		}
	}
	if _, err := mcpConfigText("nope", l); err == nil {
		t.Error("unknown client accepted")
	}
	if cmd := l.addCommand(agents[0]); strings.Join(cmd[:7], " ") != "claude mcp add --scope user dna --" || cmd[7] != l.Bin {
		t.Errorf("claude command = %q", cmd)
	}
	if link := l.cursorLink(); !strings.HasPrefix(link, "cursor://anysphere.cursor-deeplink/mcp/install?name=dna&config=") {
		t.Errorf("cursor link = %s", link)
	}
}

func TestMCPConfigCommand(t *testing.T) {
	r := run(t, nil, nil, "", "mcp", "config", "--client", "codex", "--env", "ote", "--read-only")
	if r.code != 0 || !strings.Contains(r.stdout, "args = ['mcp', 'serve', '--env', 'ote', '--read-only']") {
		t.Errorf("codex config: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	r = run(t, nil, nil, "", "mcp", "config", "--client", "nope")
	if r.code != ExitUsage {
		t.Errorf("unknown client should be a usage error, got %d", r.code)
	}
	r = run(t, nil, nil, "", "mcp", "config")
	for _, want := range []string{"# Claude Code", "# Cursor", "# Zed", "# opencode", "# Credentials"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("all clients missing %q", want)
		}
	}
}

func TestInstallConfigKeepsOtherSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark","mcpServers":{"other":{"command":"x"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := agentInfo{ID: "cursor", Name: "Cursor", Kind: kindJSON, Root: "mcpServers", Files: map[string]string{"": path}}
	l := mcpLaunch{Bin: "dna", Args: []string{"mcp", "serve"}}
	if !a.writable() {
		t.Fatal("JSON agent with a file should be writable")
	}
	got, backup, err := l.installConfig(a)
	if err != nil || got != path || backup != path+".bak" {
		t.Fatalf("install: %s %s %v", got, backup, err)
	}
	var cfg map[string]any
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	servers := cfg["mcpServers"].(map[string]any)
	if cfg["theme"] != "dark" || servers["other"] == nil || servers["dna"] == nil {
		t.Errorf("config after install: %s", b)
	}
	if old, _ := os.ReadFile(backup); !strings.Contains(string(old), `"other"`) || strings.Contains(string(old), `"dna"`) {
		t.Errorf("backup = %s", old)
	}
	// A file with comments (not plain JSON) is left alone.
	_ = os.WriteFile(path, []byte("// my settings\n{}"), 0o644)
	if _, _, err := l.installConfig(a); err == nil {
		t.Error("non-JSON file was overwritten")
	}
	if b, _ := os.ReadFile(path); string(b) != "// my settings\n{}" {
		t.Errorf("file changed: %s", b)
	}
	// TOML: appended once, refused the second time.
	toml := agentInfo{ID: "codex", Name: "Codex", Kind: kindTOML, Files: map[string]string{"": filepath.Join(dir, "config.toml")}}
	if _, _, err := l.installConfig(toml); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.installConfig(toml); err == nil {
		t.Error("second TOML install should be refused")
	}
}

func TestShellConnectAgent(t *testing.T) {
	s := testShell(t, nil, 110)
	keys(s, "down", "down", "down", "down", "down", "down") // 6: Connect an AI agent
	if v := screen(s); !strings.Contains(v, "› 6  Connect an AI agent") {
		t.Fatalf("quick action not selected:\n%s", v)
	}
	keys(s, "enter")
	picker, ok := s.top().(*menu)
	if !ok {
		t.Fatalf("agent picker not open, top = %T", s.top())
	}
	v := screen(s)
	for _, want := range []string{"Claude Code", "ChatGPT", "Cursor", "Claude Desktop", "OpenAI Codex", "more"} {
		if !strings.Contains(v, want) {
			t.Errorf("picker missing %q:\n%s", want, v)
		}
	}
	if len(picker.visible(s)) < 20 {
		t.Errorf("expected 20+ agents, got %d", len(picker.visible(s)))
	}

	keys(s, "enter") // Claude Code
	st, ok := s.top().(*agentSetup)
	if !ok || st.ag.ID != "claude-code" {
		t.Fatalf("setup not open, top = %T", s.top())
	}
	v = screen(s)
	for _, want := range []string{"Connect Claude Code", "OTE", "claude mcp add --scope user dna --", "mcp serve --env ote", "comes from this terminal", "Copy the setup", "Ready-made prompts"} {
		if !strings.Contains(v, want) {
			t.Errorf("setup missing %q:\n%s", want, v)
		}
	}
	keys(s, "right") // production
	if v := screen(s); !strings.Contains(v, "--env production") {
		t.Errorf("environment change not reflected:\n%s", v)
	}
	keys(s, "down", "left") // access: read-only → wraps to everything... then read-only
	keys(s, "esc")
	if _, ok := s.top().(*menu); !ok {
		t.Fatal("esc should return to the agent picker")
	}

	// ChatGPT explains why and offers Codex and prompts.
	for _, r := range "chatgpt" {
		keys(s, string(r))
	}
	keys(s, "enter")
	v = screen(s)
	for _, want := range []string{"cannot start programs", "OpenAI Codex", "Ready-made prompts"} {
		if !strings.Contains(v, want) {
			t.Errorf("ChatGPT screen missing %q:\n%s", want, v)
		}
	}
	keys(s, "enter") // Set up OpenAI Codex instead
	if st, ok := s.top().(*agentSetup); !ok || st.ag.ID != "codex" {
		t.Fatalf("Codex setup not opened, top = %T", s.top())
	}
	if v := screen(s); !strings.Contains(v, "[mcp_servers.dna]") {
		t.Errorf("Codex setup should show the TOML:\n%s", v)
	}
	keys(s, "esc")

	// Prompts are copied and shown.
	keys(s, "down")
	var copied string
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = func(string) error { return nil } }()
	keys(s, "enter") // Ready-made prompts to paste into ChatGPT
	if _, ok := s.top().(*menu); !ok {
		t.Fatalf("prompt menu not open, top = %T", s.top())
	}
	keys(s, "enter")
	if !strings.Contains(copied, "llms.txt") {
		t.Errorf("prompt not copied: %q", copied)
	}
}
