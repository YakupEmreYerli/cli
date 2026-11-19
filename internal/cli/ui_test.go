package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// runTTY runs the CLI as if a person sat at a terminal (stdin, stdout and
// stderr are terminals).
func runTTY(t *testing.T, api *fakeAPI, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	base := map[string]string{
		"DNA_CONFIG":      filepath.Join(t.TempDir(), "config.json"),
		"DNA_RESELLER_ID": testReseller,
		"DNA_API_KEY":     "k",
		"COLORTERM":       "truecolor",
	}
	if api != nil {
		base["DNA_BASE_URL"] = api.srv.URL
	}
	for k, v := range env {
		base[k] = v
	}
	app := &App{
		In: strings.NewReader(stdin), Out: &out, Err: &errb,
		Getenv:   func(k string) string { return base[k] },
		Build:    BuildInfo{Version: "test"},
		StdinTTY: true, StdoutTTY: true, StderrTTY: true, Width: 100,
	}
	code := app.Run(context.Background(), args)
	return result{code, out.String(), errb.String()}
}

const searchJSON = `{"success":true,"info":{"domainName":"example.com","status":"AVAILABLE","price":9.99,"currency":"USD","period":1}}`

func TestHumanViewOnTerminal(t *testing.T) {
	api := newFakeAPI(t)
	api.json("POST /api/v1/domains/search", searchJSON)
	r := runTTY(t, api, nil, "", "domain", "search", "example.com")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "is available") || !strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("expected the colored human view:\n%s", r.stdout)
	}
	// NO_COLOR keeps the layout but drops the colors.
	r = runTTY(t, api, map[string]string{"NO_COLOR": "1"}, "", "domain", "search", "example.com")
	if !strings.Contains(r.stdout, "is available") || strings.Contains(r.stdout+r.stderr, "\x1b[") {
		t.Errorf("NO_COLOR output has escape sequences or lost the view:\n%q", r.stdout)
	}
}

// Agents often run commands in a pseudo-terminal. They must still get plain
// JSON, never a prompt and never a spinner.
func TestAgentModeOnTerminal(t *testing.T) {
	api := newFakeAPI(t)
	api.json("POST /api/v1/domains/search", searchJSON)
	for _, agentVar := range []string{"AI_AGENT", "CLAUDECODE", "DNA_AGENT"} {
		r := runTTY(t, api, map[string]string{agentVar: "1"}, "", "domain", "search", "example.com")
		if r.code != 0 || decode(t, r.stdout)["success"] != true {
			t.Errorf("%s: want JSON, got exit %d:\n%s", agentVar, r.code, r.stdout)
		}
		if strings.Contains(r.stdout+r.stderr, "\x1b[") {
			t.Errorf("%s: escape sequences in agent output: %q %q", agentVar, r.stdout, r.stderr)
		}
	}
	// A billable command must not wait for an answer: exit 9 right away.
	r := runTTY(t, api, map[string]string{"AI_AGENT": "some-agent"}, "y\n", "domain", "renew", "example.com", "--period", "1")
	if r.code != ExitConfirmation {
		t.Errorf("agent renew: exit %d, want %d (stderr %s)", r.code, ExitConfirmation, r.stderr)
	}
	if !strings.Contains(r.stderr, `"confirmation_required"`) {
		t.Errorf("agent error should be JSON: %s", r.stderr)
	}
	// DNA_AGENT=0 turns detection off again.
	r = runTTY(t, api, map[string]string{"AI_AGENT": "x", "DNA_AGENT": "0"}, "", "domain", "search", "example.com")
	if !strings.Contains(r.stdout, "is available") {
		t.Errorf("DNA_AGENT=0 should restore the human view:\n%s", r.stdout)
	}
}

func TestPipeOutputNeverColored(t *testing.T) {
	api := newFakeAPI(t)
	api.json("POST /api/v1/domains/search", searchJSON)
	plain := run(t, api, nil, "", "domain", "search", "example.com")
	r := run(t, api, map[string]string{"FORCE_COLOR": "1", "CLICOLOR_FORCE": "1", "COLORTERM": "truecolor"}, "", "domain", "search", "example.com")
	if r.stdout != plain.stdout || strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("piped JSON must not change with FORCE_COLOR, got %q", r.stdout)
	}
	r = run(t, api, map[string]string{"FORCE_COLOR": "1"}, "", "domain", "info")
	if strings.Contains(r.stderr, "\x1b[") || !strings.HasPrefix(r.stderr, `{"error":`) {
		t.Errorf("piped error must be plain JSON: %q", r.stderr)
	}
}

func TestHumanConfirmationAndError(t *testing.T) {
	api := newFakeAPI(t)
	r := runTTY(t, api, nil, "n\n", "domain", "renew", "example.com", "--period", "1")
	if r.code != ExitConfirmation {
		t.Fatalf("exit %d", r.code)
	}
	for _, want := range []string{"BILLABLE", "Renew a domain", "Continue?", "Canceled, nothing was sent"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("confirmation missing %q:\n%s", want, r.stderr)
		}
	}
	if len(api.all()) != 0 {
		t.Error("declined request was sent")
	}
	r = runTTY(t, api, nil, "", "domain", "info")
	if r.code != ExitUsage || !strings.Contains(r.stderr, "Invalid input") || !strings.Contains(r.stderr, "domainName") {
		t.Errorf("human error: exit %d\n%s", r.code, r.stderr)
	}
}

func TestHumanDryRun(t *testing.T) {
	r := runTTY(t, nil, nil, "", "domain", "renew", "example.com", "--period", "2", "--dry-run")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{"DRY RUN", "nothing was sent", "BILLABLE", "POST", "/api/v1/domains/renew", `"period"`} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("dry run missing %q:\n%s", want, r.stdout)
		}
	}
	// On a pipe the dry run stays JSON.
	r = run(t, nil, nil, "", "domain", "renew", "example.com", "--period", "2", "--dry-run")
	if decode(t, r.stdout)["dryRun"] != true {
		t.Errorf("piped dry run: %s", r.stdout)
	}
}

func TestBareDnaWithoutTerminal(t *testing.T) {
	// No real terminal (and agents): `dna` prints the help, never the shell.
	for _, env := range []map[string]string{nil, {"AI_AGENT": "x"}} {
		r := run(t, nil, env, "")
		if r.code != 0 || !strings.Contains(r.stdout, "Usage:") {
			t.Errorf("piped dna should print the help: exit %d\n%s", r.code, r.stdout)
		}
		r = runTTY(t, nil, env, "")
		if r.code != 0 || !strings.Contains(ansiStrip(r.stdout), "Usage:") {
			t.Errorf("dna without a real stdin should print the help: exit %d\n%s", r.code, r.stdout)
		}
	}
	r := runTTY(t, nil, nil, "", "domain", "--help")
	if !strings.Contains(r.stdout, "\x1b[") || !strings.Contains(ansiStrip(r.stdout), "Available Commands:") {
		t.Errorf("help on a terminal should be colored:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout, "\x1b[1;38;2;28;204;153mSearch, register") {
		t.Error("a sentence ending with a colon was styled as a heading")
	}
}

func ansiStrip(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && !(s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z') {
				i++
			}
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}
