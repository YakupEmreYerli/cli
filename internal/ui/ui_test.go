package ui

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDetectLevel(t *testing.T) {
	cases := []struct {
		name string
		tty  bool
		env  map[string]string
		want Level
	}{
		{"pipe", false, nil, NoColor},
		{"pipe forced", false, map[string]string{"FORCE_COLOR": "1", "COLORTERM": "truecolor"}, TrueColor},
		{"no color wins", true, map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, NoColor},
		{"dumb", true, map[string]string{"TERM": "dumb"}, NoColor},
		{"truecolor", true, map[string]string{"COLORTERM": "24bit"}, TrueColor},
		{"force zero", false, map[string]string{"FORCE_COLOR": "0"}, NoColor},
	}
	for _, c := range cases {
		if got := DetectLevel(c.tty, env(c.env)); got != c.want {
			t.Errorf("%s: level %d, want %d", c.name, got, c.want)
		}
	}
}

func TestWidthAndTruncate(t *testing.T) {
	p := Painter{Level: TrueColor}
	if w := Width(p.Bold("abc")); w != 3 {
		t.Errorf("styled width = %d", w)
	}
	if w := Width("域名"); w != 4 {
		t.Errorf("CJK width = %d", w)
	}
	if got := Truncate("example-domain.com", 8); got != "example…" || Width(got) != 8 {
		t.Errorf("truncate = %q", got)
	}
	if got := Truncate("域名域名", 5); Width(got) > 5 {
		t.Errorf("CJK truncate too wide: %q", got)
	}
	if got := PadLeft(p.Bold("7"), 3); Width(got) != 3 || !strings.HasPrefix(got, "  ") {
		t.Errorf("pad left = %q", got)
	}
}

func TestTruncateStyled(t *testing.T) {
	p := Painter{Level: TrueColor}
	s := p.Bold("hello") + " " + p.Fg(Green, "wonderful world")
	got := TruncateStyled(s, 10)
	if Width(got) != 10 || !strings.HasPrefix(Strip(got), "hello wond") && Strip(got) != "hello won…" {
		t.Errorf("truncated = %q (%q)", got, Strip(got))
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Error("styling not reset after truncation")
	}
	if TruncateStyled("short", 10) != "short" {
		t.Error("short text changed")
	}
}

func TestToneOf(t *testing.T) {
	for s, want := range map[string]Tone{
		"Active": Good, "AVAILABLE": Good, "NOTAVAILABLE": Bad, "PendingDelete": Bad,
		"PendingRestore": Warn, "WaitingForDocument": Warn, "Failed": Bad, "something": Neutral,
	} {
		if got := ToneOf(s); got != want {
			t.Errorf("ToneOf(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestPlainPainterAddsNothing(t *testing.T) {
	var p Painter
	if p.Bold("x") != "x" || p.Fg(Red, "x") != "x" || p.Muted("x") != "x" {
		t.Error("zero painter must not add escape sequences")
	}
	if p.Badge(Amber, "BILLABLE") != "[BILLABLE]" {
		t.Errorf("plain badge = %q", p.Badge(Amber, "BILLABLE"))
	}
}

func TestColorizeJSONKeepsText(t *testing.T) {
	in := []byte("{\n  \"a\": \"x\\\"y\",\n  \"b\": [1, -2.5e3, true, null],\n  \"c\": {}\n}\n")
	out := Painter{Level: Color256}.ColorizeJSON(in)
	if string(out) == string(in) {
		t.Fatal("no colors added")
	}
	if Strip(string(out)) != string(in) {
		t.Errorf("text changed:\n%s", Strip(string(out)))
	}
	if got := (Painter{}).ColorizeJSON(in); string(got) != string(in) {
		t.Error("zero painter changed JSON")
	}
}

func TestTableFitsWidth(t *testing.T) {
	tb := Table{Headers: []string{"NAME", "VALUE"}, Rows: [][]Cell{
		{{Text: "www"}, {Text: strings.Repeat("v", 120)}},
		{{Text: "mail"}, {Text: "short"}},
	}}
	out := Painter{}.Render(tb, 40)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if Width(line) > 40 {
			t.Errorf("line wider than 40: %q", line)
		}
	}
	if !strings.Contains(out, "…") {
		t.Error("long value not truncated")
	}
}
