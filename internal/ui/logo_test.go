package ui

import (
	"strings"
	"testing"
)

func TestLogoSizes(t *testing.T) {
	p := Painter{Level: TrueColor}
	for _, width := range []int{0, 120, 80, 60, 30} {
		rows := p.Logo(width)
		if len(rows) == 0 {
			t.Fatalf("width %d: empty logo", width)
		}
		for _, r := range rows {
			if width > 0 && Width(r) > width {
				t.Errorf("width %d: row too wide (%d): %s", width, Width(r), Strip(r))
			}
		}
		t.Logf("width %d:\n%s", width, strings.Join(rows, "\n"))
	}
	if w := LogoWidth(); w < 90 || w > 120 {
		t.Errorf("big logo width %d", w)
	}
	// Every big glyph row of a letter has the same width.
	for r, g := range bigGlyphs {
		for _, row := range g {
			if Width(row) != Width(g[0]) {
				t.Errorf("glyph %q has uneven rows", r)
			}
		}
	}
	for r, g := range smallGlyphs {
		for _, row := range g {
			if Width(row) != Width(g[0]) {
				t.Errorf("small glyph %q has uneven rows", r)
			}
		}
	}
}
