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
