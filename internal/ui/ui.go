// Package ui draws the human side of dna: colors, badges, blocks, tables and
// the spinner. It is purely cosmetic and only used when a person is looking at
// a terminal. Machine output (JSON on a pipe, errors as JSON) never passes
// through it, so agents and scripts see exactly the same bytes as before.
package ui

import (
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Level is how many colors a stream can show.
type Level int

const (
	NoColor   Level = iota // plain text
	Basic                  // the 16 ANSI colors
	Color256               // xterm 256 colors
	TrueColor              // 24-bit color
)

// DetectLevel decides the color level of a stream from whether it is a
// terminal and the usual environment conventions: NO_COLOR disables colors,
// FORCE_COLOR / CLICOLOR_FORCE enable them even on a pipe, TERM=dumb disables
// them, COLORTERM and known terminals select 24-bit color.
func DetectLevel(tty bool, getenv func(string) string) Level {
	if getenv("NO_COLOR") != "" {
		return NoColor
	}
	forced := false
	for _, k := range []string{"FORCE_COLOR", "CLICOLOR_FORCE"} {
		if v := strings.ToLower(getenv(k)); v != "" && v != "0" && v != "false" {
			forced = true
		}
	}
	if !forced && (!tty || getenv("TERM") == "dumb") {
		return NoColor
	}
	switch strings.ToLower(getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return TrueColor
	}
	if getenv("WT_SESSION") != "" || runtime.GOOS == "windows" {
		return TrueColor // Windows Terminal and the Windows 10+ console
	}
	switch getenv("TERM_PROGRAM") {
	case "iTerm.app", "vscode", "WezTerm", "ghostty", "Hyper":
		return TrueColor
	}
	if strings.Contains(getenv("TERM"), "256") {
		return Color256
	}
	return Basic
}

// Color is one palette entry in the three color depths.
type Color struct {
	R, G, B uint8
	C256    int // xterm-256 index
	Basic   int // 16-color SGR code (30-37, 90-97)
}

// The palette is the Domain Name API brand (domainnameapi.com): the green of
// the logo, the deep navy of buttons and headings and the slate of body
// text. Navy is only used as a background (white text), because navy text
// disappears on the dark backgrounds most terminals use; NavyTint is its
// readable counterpart on both dark and light backgrounds.
var (
	Green     = Color{0x1C, 0xCC, 0x99, 42, 32}  // brand green #1CCC99
	GreenDeep = Color{0x0F, 0xA9, 0x7C, 36, 32}  // brand green, darker #0FA97C
	Accent    = Green                            // commands, highlights
	Navy      = Color{0x21, 0x2A, 0x4D, 17, 44}  // brand navy #212A4D (backgrounds)
	NavyTint  = Color{0x8B, 0x95, 0xC9, 104, 94} // navy, readable as text
	Amber     = Color{0xFE, 0xC3, 0x52, 221, 33} // attention
	Red       = Color{0xF2, 0x6B, 0x5B, 203, 31} // failure, danger
	Muted     = Color{0x8A, 0x93, 0xB0, 103, 90} // secondary text (slate)
	ink       = Color{0x21, 0x2A, 0x4D, 17, 30}  // brand navy, text on light badges
	white     = Color{0xFF, 0xFF, 0xFF, 231, 97} // text on dark badges
)

// Painter applies styles for one stream. The zero value paints nothing.
type Painter struct {
	Level Level
}

// On reports whether the painter emits escape sequences.
func (p Painter) On() bool { return p.Level > NoColor }

func (p Painter) code(c Color, bg bool) string {
	base := 38
	if bg {
		base = 48
	}
	switch p.Level {
	case TrueColor:
		return strconv.Itoa(base) + ";2;" + strconv.Itoa(int(c.R)) + ";" + strconv.Itoa(int(c.G)) + ";" + strconv.Itoa(int(c.B))
	case Color256:
		return strconv.Itoa(base) + ";5;" + strconv.Itoa(c.C256)
	}
	if bg {
		return strconv.Itoa(c.Basic + 10)
	}
	return strconv.Itoa(c.Basic)
}

func (p Painter) wrap(s string, codes ...string) string {
	if !p.On() || s == "" {
		return s
	}
	return "\x1b[" + strings.Join(codes, ";") + "m" + s + "\x1b[0m"
}

// Fg colors s.
func (p Painter) Fg(c Color, s string) string { return p.wrap(s, p.code(c, false)) }

// BoldFg colors s and makes it bold.
func (p Painter) BoldFg(c Color, s string) string { return p.wrap(s, "1", p.code(c, false)) }

// Bold makes s bold.
func (p Painter) Bold(s string) string { return p.wrap(s, "1") }

// Muted renders secondary text.
func (p Painter) Muted(s string) string { return p.Fg(Muted, s) }

// Accent renders text in the brand color.
func (p Painter) Accent(s string) string { return p.Fg(Accent, s) }

// Command renders a command the user can type.
func (p Painter) Command(s string) string { return p.Fg(Accent, s) }

// Badge renders a short label on a colored background: " BILLABLE ".
// Without colors it falls back to [BILLABLE].
func (p Painter) Badge(c Color, s string) string {
	if !p.On() {
		return "[" + s + "]"
	}
	fg := ink
	if c == Red || c == Navy {
		fg = white
	}
	if p.Level == Basic && c == Navy {
		// basic 16 colors have no navy; blue is the closest
		return p.wrap(" "+s+" ", "1", "44", "97")
	}
	return p.wrap(" "+s+" ", "1", p.code(c, true), p.code(fg, false))
}

// Tone classifies a value for coloring.
type Tone int

const (
	Neutral Tone = iota
	Good
	Warn
	Bad
	Info
)

// Color returns the palette color of a tone.
func (t Tone) Color() Color {
	switch t {
	case Good:
		return Green
	case Warn:
		return Amber
	case Bad:
		return Red
	case Info:
		return Accent
	}
	return Muted
}

// Toned colors s with the tone's color (neutral text stays as is).
func (p Painter) Toned(t Tone, s string) string {
	if t == Neutral {
		return s
	}
	return p.Fg(t.Color(), s)
}

// Status renders a status word with a colored dot: "● Active".
func (p Painter) Status(s string) string {
	t := ToneOf(s)
	if t == Neutral {
		return s
	}
	return p.Fg(t.Color(), "● "+s)
}

// StatusWidth is the display width Status adds to s.
func StatusWidth(s string) int {
	if ToneOf(s) == Neutral {
		return Width(s)
	}
	return Width(s) + 2
}

// ToneOf guesses the tone of a status word such as "Active",
// "PendingDelete", "NOTAVAILABLE" or "Failed".
func ToneOf(status string) Tone {
	s := strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(status))
	switch s {
	case "active", "success", "succeeded", "approved", "signed", "available", "ok", "ordered", "completed",
		"issued", "valid", "verified", "enabled", "running", "done", "add":
		return Good
	case "pendingdelete", "deleted", "failed", "failure", "rejected", "canceled", "cancelled", "transferredout",
		"transferrejected", "expired", "notavailable", "unavailable", "taken", "error", "revoked", "removed",
		"incase", "unsupportedtld", "suspended", "invalid", "subtract":
		return Bad
	case "inprogress", "processing", "removing", "modificationpending", "confirmationemailsend", "refunded", "none":
		return Warn
	}
	if strings.HasPrefix(s, "pending") || strings.HasPrefix(s, "waiting") {
		return Warn
	}
	return Neutral
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

// Strip removes ANSI escape sequences.
func Strip(s string) string { return ansi.ReplaceAllString(s, "") }

// Width is the number of terminal cells s occupies (escape sequences
// excluded, East Asian wide characters count twice).
func Width(s string) int {
	if strings.IndexByte(s, 0x1b) >= 0 {
		s = Strip(s)
	}
	n := 0
	for _, r := range s {
		n += RuneWidth(r)
	}
	return n
}

// RuneWidth is the number of terminal cells r occupies.
func RuneWidth(r rune) int {
	switch {
	case r == 0 || r < 32 || (r >= 0x7f && r < 0xa0):
		return 0
	case r >= 0x300 && r <= 0x36f, r >= 0x200b && r <= 0x200f, r == 0xfe0f, r >= 0x64b && r <= 0x65f:
		return 0 // combining marks, zero-width characters, Arabic harakat
	case r >= 0x1100 && r <= 0x115f, r >= 0x2e80 && r <= 0xa4cf && r != 0x303f, r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff, r >= 0xfe30 && r <= 0xfe4f, r >= 0xff00 && r <= 0xff60, r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f, r >= 0x1f900 && r <= 0x1f9ff, r >= 0x20000 && r <= 0x3fffd:
		return 2
	}
	return 1
}

// Truncate shortens plain text to at most w cells, ending with "…".
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	n := 0
	var sb strings.Builder
	for _, r := range s {
		rw := RuneWidth(r)
		if n+rw > w-1 {
			break
		}
		sb.WriteRune(r)
		n += rw
	}
	return sb.String() + "…"
}

// PadRight pads s (which may contain escape sequences) to w cells.
func PadRight(s string, w int) string {
	if d := w - Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// PadLeft right-aligns s in w cells.
func PadLeft(s string, w int) string {
	if d := w - Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// Block renders a titled group of lines with a colored rail:
//
//	✗ Title  meta
//	│ line
//	│ line
func (p Painter) Block(t Tone, icon, title string, lines []string) string {
	var sb strings.Builder
	c := t.Color()
	sb.WriteString("\n " + p.BoldFg(c, icon) + " " + title + "\n")
	rail := p.Fg(c, "│")
	for _, l := range lines {
		if l == "" {
			sb.WriteString(" " + rail + "\n")
			continue
		}
		sb.WriteString(" " + rail + " " + l + "\n")
	}
	return sb.String()
}

// Hint renders a suggested next step: "→ text".
func (p Painter) Hint(s string) string {
	return p.Fg(Accent, "→") + " " + s
}
