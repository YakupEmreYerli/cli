package ui

import "strings"

// Box draws lines inside a rounded border. width is the outer width; 0 fits
// the content. title (which may be styled) is set into the top border.
// Lines wider than the box are truncated (plain lines) or left as they are
// (styled lines; callers keep those short).
func (p Painter) Box(lines []string, width int, border Color, title string) []string {
	inner := 0
	for _, l := range lines {
		if w := Width(l); w > inner {
			inner = w
		}
	}
	if tw := Width(title) + 3; title != "" && tw > inner {
		inner = tw
	}
	if width > 0 {
		inner = width - 4
	}
	if inner < 1 {
		inner = 1
	}
	b := func(s string) string { return p.Fg(border, s) }
	out := make([]string, 0, len(lines)+2)
	if title != "" {
		fill := inner + 2 - Width(title) - 3
		if fill < 0 {
			fill = 0
		}
		out = append(out, b("╭─ ")+title+b(" "+strings.Repeat("─", fill)+"╮"))
	} else {
		out = append(out, b("╭"+strings.Repeat("─", inner+2)+"╮"))
	}
	for _, l := range lines {
		if Width(l) > inner && !strings.Contains(l, "\x1b") {
			l = Truncate(l, inner)
		}
		out = append(out, b("│")+" "+PadRight(l, inner)+" "+b("│"))
	}
	out = append(out, b("╰"+strings.Repeat("─", inner+2)+"╯"))
	return out
}

// SpinnerFrames are the frames of the activity indicator.
var SpinnerFrames = frames

// Shimmer renders text with a bright band sweeping across it (frame by
// frame), the way AI coding tools show that they are working.
func (p Painter) Shimmer(text string, frame int) string {
	if !p.On() {
		return text
	}
	runes := []rune(text)
	pos := frame % (len(runes) + 8)
	var sb strings.Builder
	for i, r := range runes {
		d := i - pos + 4
		if d < 0 {
			d = -d
		}
		switch {
		case d <= 1:
			sb.WriteString(p.BoldFg(Color{0xC8, 0xFF, 0xEC, 158, 97}, string(r)))
		case d <= 3:
			sb.WriteString(p.Fg(Green, string(r)))
		default:
			sb.WriteString(p.Fg(GreenDeep, string(r)))
		}
	}
	return sb.String()
}
