package config

import (
	"slices"
	"testing"
)

func TestLayoutNormalize(t *testing.T) {
	l := Layout{Left: []string{"network", "bogus"}, Center: []string{"network", "visualizer"}}
	l.normalize()
	if !slices.Equal(l.Left, []string{"network"}) || !slices.Equal(l.Center, []string{"visualizer"}) ||
		!slices.Equal(l.Right, []string{"media"}) {
		t.Fatalf("got %+v", l)
	}
}

func TestLayoutMove(t *testing.T) {
	l := Default().Layout
	l.Move(SectionNetwork, ZoneCenter, 1)
	if !slices.Equal(l.Center, []string{"visualizer", "network", "media"}) || len(l.Right) != 0 {
		t.Fatalf("got %+v", l)
	}
	l.Move(SectionVisualizer, ZoneCenter, 99)
	if !slices.Equal(l.Center, []string{"network", "media", "visualizer"}) {
		t.Fatalf("got %+v", l)
	}
}

func TestLayoutForSeedsCopy(t *testing.T) {
	c := Default()
	l := c.LayoutFor(`\\.\DISPLAY7`)
	if l == &c.Layout {
		t.Fatal("display layout is the template itself")
	}
	l.Left = append(l.Left, SectionVisualizer)
	l.LeftOffset = 40
	if len(c.Layout.Left) != 0 || c.Layout.LeftOffset != 12 {
		t.Fatalf("template changed: %+v", c.Layout)
	}
	if len(l.Right) > 0 && len(c.Layout.Right) > 0 && &l.Right[0] == &c.Layout.Right[0] {
		t.Fatal("zone slice shares its array with the template")
	}
}

func TestLayoutForKeepsEntry(t *testing.T) {
	c := Default()
	l := c.LayoutFor(`\\.\DISPLAY7`)
	l.RightOffset = 40
	if got := c.LayoutFor(`\\.\DISPLAY7`); got != l || got.RightOffset != 40 {
		t.Fatalf("got %+v, want the stored layout", got)
	}
	c.Display = `\\.\DISPLAY7`
	if got := c.DisplayLayout(); got != l {
		t.Fatal("DisplayLayout did not return the display's own layout")
	}
}

func TestLayoutForPrimary(t *testing.T) {
	c := Default()
	if got := c.LayoutFor(""); got != &c.Layout {
		t.Fatal("the primary taskbar must edit the template layout")
	}
	c.LayoutFor(`\\.\DISPLAY7`)
	if _, ok := c.Layouts[""]; ok {
		t.Fatal("the primary taskbar stored a layout of its own")
	}
}

func TestNormalizeLayouts(t *testing.T) {
	c := Config{
		Layout: Default().Layout,
		Layouts: map[string]*Layout{
			`\\.\DISPLAY7`: {Left: []string{"network", "network", "bogus"}, LeftOffset: 5000, CenterOffset: -9000, RightOffset: -1},
			`\\.\DISPLAY3`: {Right: []string{"media"}},
			"":             {Left: []string{"media"}},
		},
	}
	c.normalize()
	l, ok := c.Layouts[`\\.\DISPLAY7`]
	if !ok {
		t.Fatal("display layout dropped")
	}
	if !slices.Equal(l.Left, []string{"network"}) || !slices.Equal(l.Right, []string{"visualizer", "media"}) {
		t.Fatalf("sections not normalized: %+v", l)
	}
	if l.LeftOffset != 2000 || l.CenterOffset != -2000 || l.RightOffset != 0 {
		t.Fatalf("offsets not clamped: %+v", l)
	}
	if _, ok := c.Layouts[`\\.\DISPLAY3`]; !ok {
		t.Fatal("display layout without offsets dropped")
	}
	if _, ok := c.Layouts[""]; ok {
		t.Fatal("entry for the empty device kept")
	}
	if c.Layout.LeftOffset != 12 || !slices.Equal(c.Layout.Center, []string{"visualizer", "media"}) {
		t.Fatalf("template not normalized: %+v", c.Layout)
	}
}
