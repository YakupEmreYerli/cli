package ui

import "strings"

// The Domain Name API logo is a bold "D" with a green shadow, "omain" in
// green and "name api" underneath. The terminal logo follows it: solid
// strokes in the terminal's own text color (white on dark, ink on light),
// shadows in Domain Name API green, and "NAME API" in a green gradient.

// ANSI Shadow letters, 6 rows each.
var bigGlyphs = map[rune][6]string{
	'D': {"██████╗ ", "██╔══██╗", "██║  ██║", "██║  ██║", "██████╔╝", "╚═════╝ "},
	'O': {" ██████╗ ", "██╔═══██╗", "██║   ██║", "██║   ██║", "╚██████╔╝", " ╚═════╝ "},
	'M': {"███╗   ███╗", "████╗ ████║", "██╔████╔██║", "██║╚██╔╝██║", "██║ ╚═╝ ██║", "╚═╝     ╚═╝"},
	'A': {" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
	'I': {"██╗", "██║", "██║", "██║", "██║", "╚═╝"},
	'N': {"███╗   ██╗", "████╗  ██║", "██╔██╗ ██║", "██║╚██╗██║", "██║ ╚████║", "╚═╝  ╚═══╝"},
	'E': {"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"},
	'P': {"██████╗ ", "██╔══██╗", "██████╔╝", "██╔═══╝ ", "██║     ", "╚═╝     "},
	' ': {"  ", "  ", "  ", "  ", "  ", "  "},
}

// Calvin S letters, 3 rows each: a compact logo for narrower terminals.
var smallGlyphs = map[rune][3]string{
	'D': {"╔╦╗", " ║║", "═╩╝"},
	'o': {"┌─┐", "│ │", "└─┘"},
	'm': {"┌┬┐", "│││", "┴ ┴"},
	'a': {"┌─┐", "├─┤", "┴ ┴"},
	'i': {"┬", "│", "┴"},
	'n': {"┌┐┌", "│││", "┘└┘"},
	'N': {"╔╗╔", "║║║", "╝╚╝"},
	'e': {"┌─┐", "├┤ ", "└─┘"},
	'A': {"╔═╗", "╠═╣", "╩ ╩"},
	'P': {"╔═╗", "╠═╝", "╩  "},
	'I': {"╦", "║", "╩"},
	' ': {"  ", "  ", "  "},
}

// Brand greens for the gradient of "NAME API", top to bottom.
var logoGradient = []Color{
	{0x3B, 0xE3, 0xB0, 49, 92},
	{0x1C, 0xCC, 0x99, 42, 32},
	{0x1C, 0xCC, 0x99, 42, 32},
	{0x12, 0xB3, 0x85, 36, 32},
	{0x0F, 0xA9, 0x7C, 36, 32},
	{0x0E, 0x8F, 0x69, 29, 32},
}

// LogoWidth is the width of the big one-line logo.
func LogoWidth() int { return Width(strings.Join(bigRows("DOMAIN NAME API")[:1], "")) }

func bigRows(text string) []string {
	rows := make([]string, 6)
	for _, r := range text {
		g := bigGlyphs[r]
		for row := range rows {
			rows[row] += g[row]
		}
	}
	return rows
}

// Logo draws "DOMAIN NAME API" for a terminal of the given width: the big
// ANSI Shadow logo when it fits, a compact one otherwise, and a one-line
// wordmark on very narrow terminals.
func (p Painter) Logo(width int) []string {
	if width == 0 || width >= LogoWidth()+4 {
		return p.bigLogo()
	}
	if width >= 44 {
		return p.smallLogo()
	}
	return []string{p.Bold("◆ Domain ") + p.BoldFg(Green, "Name API")}
}

func (p Painter) bigLogo() []string {
	domain := bigRows("DOMAIN")
	nameAPI := bigRows("NAME API")
	out := make([]string, 6)
	for row := 0; row < 6; row++ {
		var sb strings.Builder
		sb.WriteString(p.paintGlyphRow(domain[row], func(s string) string { return p.Bold(s) }, func(s string) string { return p.Fg(Green, s) }))
		sb.WriteString("  ")
		g := logoGradient[row]
		sb.WriteString(p.paintGlyphRow(nameAPI[row], func(s string) string { return p.BoldFg(g, s) }, func(s string) string { return p.Fg(GreenDeep, s) }))
		out[row] = strings.TrimRight(sb.String(), " ")
	}
	return out
}

// paintGlyphRow colors the solid blocks and the shadow strokes of a row.
func (p Painter) paintGlyphRow(row string, solid, shadow func(string) string) string {
	var sb, run strings.Builder
	kind := 0 // 0 space, 1 solid, 2 shadow
	flush := func() {
		s := run.String()
		run.Reset()
		switch kind {
		case 1:
			sb.WriteString(solid(s))
		case 2:
			sb.WriteString(shadow(s))
		default:
			sb.WriteString(s)
		}
	}
	for _, r := range row {
		k := 2
		switch r {
		case ' ':
			k = 0
		case '█':
			k = 1
		}
		if k != kind && run.Len() > 0 {
			flush()
		}
		kind = k
		run.WriteRune(r)
	}
	flush()
	return sb.String()
}

func (p Painter) smallLogo() []string {
	out := make([]string, 3)
	for row := 0; row < 3; row++ {
		var domain, rest strings.Builder
		for _, r := range "Domain" {
			domain.WriteString(smallGlyphs[r][row])
		}
		for _, r := range " Name API" {
			rest.WriteString(smallGlyphs[r][row])
		}
		out[row] = p.Bold(domain.String()) + p.BoldFg(Green, rest.String())
	}
	return out
}
