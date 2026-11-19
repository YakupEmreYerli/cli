package ui

import (
	"strings"
	"unicode/utf8"
)

// TruncateStyled shortens s, which may contain escape sequences, to w cells,
// ending with "…". The escape sequences are kept, so colors stay intact.
func TruncateStyled(s string, w int) string {
	if Width(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	var sb strings.Builder
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				j++
			}
			if j > len(s) {
				j = len(s)
			}
			sb.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if rw := RuneWidth(r); n+rw > w-1 {
			break
		} else {
			n += rw
		}
		sb.WriteRune(r)
		i += size
	}
	sb.WriteString("…")
	if strings.IndexByte(s, 0x1b) >= 0 {
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
}

// Box draws lines inside a rounded border. width is the outer width; 0 fits
// the content. title (which may be styled) is set into the top border.
// Lines wider than the box are truncated, so a narrow terminal never wraps.
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
		if Width(l) > inner {
			l = TruncateStyled(l, inner)
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
