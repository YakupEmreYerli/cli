package ui

import "bytes"

// ColorizeJSON adds colors to encoded JSON for a terminal: keys in brand
// green, strings in navy tint, numbers amber, true/false/null coral and
// punctuation muted. The text itself is unchanged, so copying it from the
// terminal gives valid JSON.
func (p Painter) ColorizeJSON(b []byte) []byte {
	if !p.On() {
		return b
	}
	var out bytes.Buffer
	out.Grow(len(b) * 2)
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(b) {
				if b[j] == '\\' {
					j += 2
					continue
				}
				if b[j] == '"' {
					break
				}
				j++
			}
			if j >= len(b) {
				j = len(b) - 1
			}
			tok := string(b[i : j+1])
			k := j + 1
			for k < len(b) && (b[k] == ' ' || b[k] == '\n' || b[k] == '\t' || b[k] == '\r') {
				k++
			}
			if k < len(b) && b[k] == ':' {
				out.WriteString(p.Fg(Accent, tok))
			} else {
				out.WriteString(p.Fg(NavyTint, tok))
			}
			i = j + 1
		case c == '-' || (c >= '0' && c <= '9'):
			j := i
			for j < len(b) && bytes.IndexByte([]byte("+-.0123456789eE"), b[j]) >= 0 {
				j++
			}
			out.WriteString(p.Fg(Amber, string(b[i:j])))
			i = j
		case c == 't' || c == 'f' || c == 'n':
			j := i
			for j < len(b) && b[j] >= 'a' && b[j] <= 'z' {
				j++
			}
			out.WriteString(p.Fg(Red, string(b[i:j])))
			i = j
		case c == '{' || c == '}' || c == '[' || c == ']' || c == ':' || c == ',':
			out.WriteString(p.Muted(string(c)))
			i++
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.Bytes()
}
