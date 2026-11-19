package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// AI agents (MCP clients) dna can be connected to, with exactly how. Used by
// `dna mcp config --client` and by the shell's "Connect an AI agent".

type agentKind int

const (
	kindCommand agentKind = iota // a CLI command adds the server (claude mcp add)
	kindJSON                     // a JSON config file with a server map
	kindTOML                     // a TOML config file (Codex)
	kindYAML                     // a YAML config file (Continue)
	kindSteps                    // settings screens; the user pastes JSON or a command
	kindChat                     // a chat without local tools: ready-made prompts only
)

type agentInfo struct {
	ID      string // --client value
	Name    string
	Desc    string // one line in the picker
	Kind    agentKind
	Root    string            // JSON key that holds the servers (mcpServers, servers, mcp)
	Files   map[string]string // config file per GOOS ("" = any); ~ and %APPDATA% are expanded
	Steps   []string          // where to paste (kindSteps), or extra notes
	Then    string            // how to start using it afterwards
	Exe     string            // program to detect on this computer
	Popular bool
}

var agents = []agentInfo{
	{ID: "claude-code", Name: "Claude Code", Desc: "Anthropic's coding agent in the terminal and IDEs", Kind: kindCommand, Exe: "claude", Popular: true,
		Then: `Start claude anywhere and ask: "Use dna to show my balance and list my domains."`},
	{ID: "chatgpt", Name: "ChatGPT", Desc: "OpenAI's assistant: use Codex for local tools, or a ready-made prompt", Kind: kindChat, Popular: true},
	{ID: "cursor", Name: "Cursor", Desc: "AI code editor", Kind: kindJSON, Root: "mcpServers", Exe: "cursor", Popular: true,
		Files: map[string]string{"": "~/.cursor/mcp.json"},
		Then:  "Open Cursor Settings → MCP: dna should be listed and enabled. Then ask the agent in chat."},
	{ID: "claude-desktop", Name: "Claude Desktop", Desc: "the Claude app for Windows and macOS", Kind: kindJSON, Root: "mcpServers",
		Files: map[string]string{"windows": `%APPDATA%\Claude\claude_desktop_config.json`, "darwin": "~/Library/Application Support/Claude/claude_desktop_config.json", "": "~/.config/Claude/claude_desktop_config.json"},
		Then:  "Quit Claude Desktop completely and open it again; dna appears under the tools (search and tools) button."},
	{ID: "codex", Name: "OpenAI Codex", Desc: "OpenAI's coding agent (CLI and IDE), included with ChatGPT plans", Kind: kindTOML, Exe: "codex", Popular: true,
		Files: map[string]string{"": "~/.codex/config.toml"},
		Then:  `Start a new Codex session and ask: "Use dna to show my balance."`},
	{ID: "vscode", Name: "VS Code (GitHub Copilot)", Desc: "Copilot Chat in agent mode", Kind: kindJSON, Root: "servers", Exe: "code",
		Files: map[string]string{"": ".vscode/mcp.json (in your project)"},
		Then:  "Open Copilot Chat, switch to Agent mode and enable the dna tools under the tools icon."},
	{ID: "windsurf", Name: "Windsurf", Desc: "Codeium's AI editor (Cascade)", Kind: kindJSON, Root: "mcpServers",
		Files: map[string]string{"": "~/.codeium/windsurf/mcp_config.json"},
		Then:  "In Cascade open the MCP servers panel and press refresh."},
	{ID: "gemini", Name: "Gemini CLI", Desc: "Google's agent in the terminal", Kind: kindJSON, Root: "mcpServers", Exe: "gemini",
		Files: map[string]string{"": "~/.gemini/settings.json"},
		Then:  "Start gemini and run /mcp to see the dna tools."},
	{ID: "zed", Name: "Zed", Desc: "editor with an agent panel", Kind: kindSteps,
		Steps: []string{"Open the Agent Panel → settings → Add Custom Server.", "Name: dna · Command:"}, Then: "The dna tools appear in the agent panel."},
	{ID: "cline", Name: "Cline", Desc: "VS Code extension", Kind: kindSteps,
		Steps: []string{"In VS Code open Cline → MCP Servers → Installed → Configure MCP Servers.", "Add this to the mcpServers in that file:"}},
	{ID: "roo", Name: "Roo Code", Desc: "VS Code extension", Kind: kindSteps,
		Steps: []string{"In Roo Code open MCP Servers → Edit Global MCP.", "Add this to the mcpServers in that file:"}},
	{ID: "continue", Name: "Continue", Desc: "open-source assistant for VS Code and JetBrains", Kind: kindYAML,
		Files: map[string]string{"": "~/.continue/config.yaml"}, Then: "Reload Continue; use the dna tools in agent mode."},
	{ID: "goose", Name: "Goose", Desc: "Block's open-source agent", Kind: kindSteps, Exe: "goose",
		Steps: []string{"Run goose configure → Add Extension → Command-line Extension.", "Name: dna · Command:"}},
	{ID: "opencode", Name: "opencode", Desc: "open-source agent in the terminal", Kind: kindJSON, Root: "mcp", Exe: "opencode",
		Files: map[string]string{"": "~/.config/opencode/opencode.json"}, Then: "Start opencode; the dna tools are available to the agent."},
	{ID: "amazonq", Name: "Amazon Q Developer", Desc: "AWS's agent in the terminal and IDEs", Kind: kindJSON, Root: "mcpServers", Exe: "q",
		Files: map[string]string{"": "~/.aws/amazonq/mcp.json"}, Then: "Start q chat; the dna tools load automatically."},
	{ID: "jetbrains", Name: "JetBrains AI Assistant", Desc: "IntelliJ IDEA, PyCharm, GoLand and others", Kind: kindSteps,
		Steps: []string{"Settings → Tools → AI Assistant → Model Context Protocol (MCP) → Add → As JSON.", "Paste:"}},
	{ID: "lmstudio", Name: "LM Studio", Desc: "local models with MCP tools", Kind: kindSteps,
		Steps: []string{"In LM Studio open the Program tab → Install → Edit mcp.json.", "Add this to the mcpServers in that file:"}},
	{ID: "warp", Name: "Warp", Desc: "AI terminal", Kind: kindSteps,
		Steps: []string{"Settings → AI → Manage MCP servers → Add.", "Paste:"}},
	{ID: "copilot-cli", Name: "GitHub Copilot CLI", Desc: "Copilot in the terminal", Kind: kindSteps, Exe: "copilot",
		Steps: []string{"Start copilot and run /mcp add (type: local / STDIO).", "Name: dna · Command:"}},
	{ID: "other", Name: "Another MCP client", Desc: "anything that runs local (stdio) MCP servers", Kind: kindSteps,
		Steps: []string{"Add a local (stdio) MCP server named dna. Most apps accept this JSON:"}},
	{ID: "chat", Name: "Claude.ai, Gemini or another chat", Desc: "no tools on your computer: ready-made prompts", Kind: kindChat},
}

func lookupAgent(id string) (agentInfo, bool) {
	for _, a := range agents {
		if a.ID == id {
			return a, true
		}
	}
	return agentInfo{}, false
}

func agentIDs() []string {
	ids := make([]string, len(agents))
	for i, a := range agents {
		ids[i] = a.ID
	}
	return ids
}

// mcpLaunch is what an agent runs to start dna's MCP server.
type mcpLaunch struct {
	Bin  string
	Args []string
}

// commandLine is the launch command as one line for a shell.
func (l mcpLaunch) commandLine() string {
	parts := []string{shellQuote(l.Bin)}
	for _, a := range l.Args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'$&|;<>()`!*?[]{}") {
		return s
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// jsonObject marshals its fields in order (command before args reads better
// than Go's alphabetical map order).
type jsonObject []jsonField

type jsonField struct {
	key   string
	value any
}

func (o jsonObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.key)
		b.Write(k)
		b.WriteByte(':')
		var v bytes.Buffer
		enc := json.NewEncoder(&v)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(f.value); err != nil {
			return nil, err
		}
		b.Write(bytes.TrimRight(v.Bytes(), "\n"))
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (l mcpLaunch) entry(a agentInfo) jsonObject {
	switch a.ID {
	case "vscode":
		return jsonObject{{"type", "stdio"}, {"command", l.Bin}, {"args", l.Args}}
	case "opencode":
		return jsonObject{{"type", "local"}, {"command", append([]string{l.Bin}, l.Args...)}, {"enabled", true}}
	}
	return jsonObject{{"command", l.Bin}, {"args", l.Args}}
}

func marshalJSON(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return strings.TrimRight(b.String(), "\n")
}

// addCommand is the CLI command that registers dna with the agent, when the
// agent has one.
func (l mcpLaunch) addCommand(a agentInfo) []string {
	switch a.ID {
	case "claude-code":
		return append([]string{"claude", "mcp", "add", "--scope", "user", "dna", "--", l.Bin}, l.Args...)
	case "vscode":
		cfg := marshalJSONCompact(jsonObject{{"name", "dna"}, {"command", l.Bin}, {"args", l.Args}})
		return []string{"code", "--add-mcp", cfg}
	}
	return nil
}

func marshalJSONCompact(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimRight(b.String(), "\n")
}

// cursorLink is Cursor's one-click install link for dna.
func (l mcpLaunch) cursorLink() string {
	cfg := marshalJSONCompact(jsonObject{{"command", l.Bin}, {"args", l.Args}})
	return "cursor://anysphere.cursor-deeplink/mcp/install?name=dna&config=" + base64.StdEncoding.EncodeToString([]byte(cfg))
}

// configFile is the agent's config file on this computer ("" if none).
func (a agentInfo) configFile() string {
	p, ok := a.Files[runtime.GOOS]
	if !ok {
		p = a.Files[""]
	}
	return p
}

// expandPath resolves ~ and %APPDATA% in a config path.
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if strings.Contains(p, "%APPDATA%") {
		p = strings.ReplaceAll(p, "%APPDATA%", os.Getenv("APPDATA"))
	}
	return filepath.FromSlash(p)
}

// writable reports whether dna can add itself to the agent's config file.
func (a agentInfo) writable() bool {
	f := a.configFile()
	return (a.Kind == kindJSON || a.Kind == kindTOML) && f != "" && !strings.Contains(f, "(")
}

// snippet is the text to paste (or the command to run) and its language.
func (l mcpLaunch) snippet(a agentInfo) (text, lang string) {
	switch a.Kind {
	case kindCommand:
		return strings.Join(quoteAll(l.addCommand(a)), " "), "bash"
	case kindJSON:
		return marshalJSON(map[string]any{a.Root: map[string]any{"dna": l.entry(a)}}), "json"
	case kindTOML:
		return l.toml(), "toml"
	case kindYAML:
		var b strings.Builder
		b.WriteString("mcpServers:\n  - name: dna\n    command: " + yamlQuote(l.Bin) + "\n    args:\n")
		for _, x := range l.Args {
			b.WriteString("      - " + yamlQuote(x) + "\n")
		}
		return strings.TrimRight(b.String(), "\n"), "yaml"
	case kindSteps:
		if strings.HasSuffix(a.Steps[len(a.Steps)-1], "Command:") {
			return l.commandLine(), "text"
		}
		if a.ID == "warp" || a.ID == "jetbrains" {
			return marshalJSON(map[string]any{"mcpServers": map[string]any{"dna": l.entry(a)}}), "json"
		}
		return marshalJSON(map[string]any{"dna": l.entry(a)}), "json"
	}
	return "", ""
}

func (l mcpLaunch) toml() string {
	parts := make([]string, len(l.Args))
	for i, a := range l.Args {
		parts[i] = tomlQuote(a)
	}
	return "[mcp_servers.dna]\ncommand = " + tomlQuote(l.Bin) + "\nargs = [" + strings.Join(parts, ", ") + "]"
}

func tomlQuote(s string) string {
	if !strings.ContainsAny(s, `'`) {
		return "'" + s + "'" // literal string: backslashes in Windows paths stay as they are
	}
	return fmt.Sprintf("%q", s)
}

func yamlQuote(s string) string {
	if strings.ContainsAny(s, `:#\ '"`) {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return s
}

func quoteAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = shellQuote(a)
	}
	return out
}

// installConfig adds dna to the agent's config file, keeping everything
// else in it. The previous file is saved next to it with a .bak suffix.
func (l mcpLaunch) installConfig(a agentInfo) (path, backup string, err error) {
	path = expandPath(a.configFile())
	old, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return path, "", err
	}
	var out []byte
	switch a.Kind {
	case kindJSON:
		cfg := map[string]any{}
		if exists && len(bytes.TrimSpace(old)) > 0 {
			if err := json.Unmarshal(old, &cfg); err != nil {
				return path, "", fmt.Errorf("%s is not plain JSON (%v); add the snippet by hand", path, err)
			}
		}
		servers, _ := cfg[a.Root].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
		}
		servers["dna"] = l.entry(a)
		cfg[a.Root] = servers
		out = []byte(marshalJSON(cfg) + "\n")
	case kindTOML:
		if exists && bytes.Contains(old, []byte("[mcp_servers.dna]")) {
			return path, "", fmt.Errorf("%s already has a [mcp_servers.dna] section; edit it by hand", path)
		}
		out = append(append([]byte{}, old...), []byte("\n"+l.toml()+"\n")...)
	default:
		return path, "", fmt.Errorf("%s has no config file dna can edit", a.Name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, "", err
	}
	mode := os.FileMode(0o600)
	if exists {
		if st, err := os.Stat(path); err == nil {
			mode = st.Mode().Perm()
		}
		backup = path + ".bak"
		if err := os.WriteFile(backup, old, 0o600); err != nil {
			return path, "", err
		}
	}
	return path, backup, os.WriteFile(path, out, mode)
}

// found reports whether the agent's program is installed here.
func (a agentInfo) found() bool {
	if a.Exe == "" {
		return false
	}
	_, err := lookPath(a.Exe)
	return err == nil
}

// dnaPath is the dna program for agents to start: the one on the PATH (a
// stable path that survives upgrades), or else this program's own path.
// Agents often start with a different PATH than your terminal, so the path
// is absolute.
// lookPath finds a program on the PATH (replaced in tests).
var lookPath = exec.LookPath

func dnaPath() string {
	if p, err := lookPath("dna"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "dna"
}

// mcpConfigText renders `dna mcp config` for one agent or for all of them.
func mcpConfigText(client string, l mcpLaunch) (string, error) {
	render := func(a agentInfo) string {
		var b strings.Builder
		b.WriteString("# " + a.Name)
		if f := a.configFile(); f != "" && (a.Kind == kindJSON || a.Kind == kindTOML || a.Kind == kindYAML) {
			b.WriteString(": " + f)
		}
		b.WriteString("\n")
		if a.Kind == kindChat {
			b.WriteString("# No local tools: use a ready-made prompt (docs/ai-agents.md#ready-made-prompts)")
			if a.ID == "chatgpt" {
				b.WriteString(", or connect OpenAI Codex: dna mcp config --client codex")
			}
			b.WriteString("\n")
			return b.String()
		}
		for _, s := range a.Steps {
			b.WriteString("# " + s + "\n")
		}
		text, _ := l.snippet(a)
		b.WriteString(text + "\n")
		if a.Then != "" {
			b.WriteString("# Then: " + a.Then + "\n")
		}
		return b.String()
	}
	if client != "" {
		a, ok := lookupAgent(client)
		if !ok {
			return "", fmt.Errorf("unknown client %q (known: %s)", client, strings.Join(agentIDs(), ", "))
		}
		return render(a), nil
	}
	ids := agentIDs()
	sort.Strings(ids)
	var b strings.Builder
	for _, id := range ids {
		a, _ := lookupAgent(id)
		if a.Kind == kindChat {
			continue
		}
		b.WriteString(render(a) + "\n")
	}
	b.WriteString("# Credentials: run `dna auth login` once, or add an \"env\" block with DNA_RESELLER_ID and DNA_API_KEY.\n")
	return b.String(), nil
}
