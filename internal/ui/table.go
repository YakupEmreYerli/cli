package ui

import "strings"

// Align is the horizontal alignment of a column.
type Align int

const (
	Left Align = iota
	Right
)

// Cell is one table cell: plain text (measured and truncated) and an
// optional style applied after truncation.
type Cell struct {
	Text  string
	Style func(string) string
	// Width overrides Width(Text) when Style adds visible characters
	// (for example the dot of a status).
	Extra int
}

// Table is a simple column-aligned table.
type Table struct {
	Headers []string
	Align   []Align
	Rows    [][]Cell
	// MinWidth is the narrowest a column shrinks to when the table does not
	// fit the terminal (default 6).
	MinWidth int
	// Indent is the left margin (default 2).
	Indent int
}

// Render draws the table with a two-space indent. width is the terminal
// width (0 = unlimited); wide columns are truncated to make the table fit.
func (p Painter) Render(t Table, width int) string {
	n := len(t.Headers)
	if n == 0 {
		return ""
	}
	w := make([]int, n)
	for i, h := range t.Headers {
		w[i] = Width(h)
	}
	for _, r := range t.Rows {
		for i := 0; i < n && i < len(r); i++ {
			if cw := Width(r[i].Text) + r[i].Extra; cw > w[i] {
				w[i] = cw
			}
		}
	}
	const gap = 2
	indent := t.Indent
	if indent == 0 {
		indent = 2
	}
	if width > 0 {
		min := t.MinWidth
		if min == 0 {
			min = 6
		}
		total := func() int {
			s := indent + gap*(n-1)
			for _, x := range w {
				s += x
			}
			return s
		}
		for total() > width {
			// shrink the widest column that can still shrink
			best := -1
			for i := range w {
				if w[i] > min && w[i] > Width(t.Headers[i]) && (best < 0 || w[i] > w[best]) {
					best = i
				}
			}
			if best < 0 {
				break
			}
			w[best]--
		}
	}
	var sb strings.Builder
	sb.WriteString(strings.Repeat(" ", indent))
	for i, h := range t.Headers {
		cell := h
		if i < n-1 || (len(t.Align) > i && t.Align[i] == Right) {
			cell = pad(h, w[i], alignOf(t, i))
		}
		sb.WriteString(p.BoldFg(Muted, cell))
		if i < n-1 {
			sb.WriteString(strings.Repeat(" ", gap))
		}
	}
	sb.WriteString("\n")
	for _, r := range t.Rows {
		var line strings.Builder
		line.WriteString(strings.Repeat(" ", indent))
		for i := 0; i < n; i++ {
			var c Cell
			if i < len(r) {
				c = r[i]
			}
			text := Truncate(c.Text, w[i]-c.Extra)
			styled := text
			if c.Style != nil && text != "" {
				styled = c.Style(text)
			}
			if i < n-1 || alignOf(t, i) == Right {
				styled = pad(styled, w[i], alignOf(t, i))
			}
			line.WriteString(styled)
			if i < n-1 {
				line.WriteString(strings.Repeat(" ", gap))
			}
		}
		sb.WriteString(strings.TrimRight(line.String(), " ") + "\n")
	}
	return sb.String()
}

func alignOf(t Table, i int) Align {
	if i < len(t.Align) {
		return t.Align[i]
	}
	return Left
}

func pad(s string, w int, a Align) string {
	if a == Right {
		return PadLeft(s, w)
	}
	return PadRight(s, w)
}

// KV renders aligned "label  value" lines with the given indent. Labels are
// muted; values are used as given (they may be styled).
func (p Painter) KV(pairs [][2]string, indent int) string {
	lw := 0
	for _, kv := range pairs {
		if w := Width(kv[0]); w > lw {
			lw = w
		}
	}
	var sb strings.Builder
	for _, kv := range pairs {
		sb.WriteString(strings.Repeat(" ", indent))
		sb.WriteString(p.Muted(PadRight(kv[0], lw)))
		sb.WriteString("  ")
		sb.WriteString(kv[1])
		sb.WriteString("\n")
	}
	return sb.String()
}
