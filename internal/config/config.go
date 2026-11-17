// Package config loads and saves %APPDATA%\wintoolbar\config.json.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// Config is the whole user configuration. Missing fields keep their defaults.
type Config struct {
	// Visible shows the widget. Hidden means no polling and no capture.
	Visible bool `json:"visible"`

	// Display is the monitor device name whose taskbar hosts the widget,
	// such as `\\.\DISPLAY7`. Empty picks the primary taskbar.
	Display string `json:"display"`

	// Layout is the layout of a display without its own entry in Layouts,
	// and the template a new entry starts from.
	Layout Layout `json:"layout"`

	// Layouts holds the layout per display device name. The values are
	// pointers so LayoutFor can hand out the stored layout itself; map
	// values are not addressable.
	Layouts map[string]*Layout `json:"layouts,omitempty"`

	Visualizer Visualizer `json:"visualizer"`
	Media      Media      `json:"media"`
	Network    Network    `json:"network"`
}

// Media configures the media player section (Windows SMTC).
type Media struct {
	Enabled bool `json:"enabled"`
	// Controls shows previous, play/pause and next buttons.
	Controls bool `json:"controls"`
	// Stop adds a stop button after next.
	Stop bool `json:"stop"`
}

// Section IDs used in Layout.
const (
	SectionVisualizer = "visualizer"
	SectionMedia      = "media"
	SectionNetwork    = "network"
)

// Sections lists every section ID.
var Sections = [...]string{SectionVisualizer, SectionMedia, SectionNetwork}

// Zone names, in Layout.Zones order.
const (
	ZoneLeft = iota
	ZoneCenter
	ZoneRight
	ZoneCount
)

// ZoneNames are the user-facing zone names.
var ZoneNames = [ZoneCount]string{"left", "center", "right"}

// Layout places sections in three taskbar zones. Each list is drawn left to
// right in its order. A section listed nowhere is placed at the end of the
// right zone; a section listed twice keeps its first place.
type Layout struct {
	// Left starts at the left edge of the taskbar, or after the app icons
	// when the taskbar icons are left-aligned.
	Left []string `json:"left"`
	// Center is centered on the taskbar.
	Center []string `json:"center"`
	// Right ends at the notification area (tray).
	Right []string `json:"right"`

	// Offsets in logical pixels: Left moves right, Right moves left,
	// Center moves right (negative moves left).
	LeftOffset   int `json:"leftOffset"`
	CenterOffset int `json:"centerOffset"`
	RightOffset  int `json:"rightOffset"`
}

// Zone returns the list for zone index z.
func (l *Layout) Zone(z int) *[]string {
	switch z {
	case ZoneLeft:
		return &l.Left
	case ZoneCenter:
		return &l.Center
	default:
		return &l.Right
	}
}

// clone copies the layout and its zone slices.
func (l Layout) clone() Layout {
	return Layout{
		Left:         slices.Clone(l.Left),
		Center:       slices.Clone(l.Center),
		Right:        slices.Clone(l.Right),
		LeftOffset:   l.LeftOffset,
		CenterOffset: l.CenterOffset,
		RightOffset:  l.RightOffset,
	}
}

// LayoutFor returns the layout of a display device name. The first call seeds
// the entry with a deep copy of Layout, so editing one display leaves the
// others alone. An empty name is the primary taskbar and uses Layout itself.
func (c *Config) LayoutFor(device string) *Layout {
	if device == "" {
		return &c.Layout
	}
	if l, ok := c.Layouts[device]; ok && l != nil {
		return l
	}
	l := c.Layout.clone()
	if c.Layouts == nil {
		c.Layouts = make(map[string]*Layout, 2)
	}
	c.Layouts[device] = &l
	return &l
}

// DisplayLayout returns the layout of the display the widget runs on.
func (c *Config) DisplayLayout() *Layout { return c.LayoutFor(c.Display) }

// Find returns the zone and index of section id, or -1, -1.
func (l *Layout) Find(id string) (zone, index int) {
	for z := range ZoneCount {
		for i, s := range *l.Zone(z) {
			if s == id {
				return z, i
			}
		}
	}
	return -1, -1
}

// Move puts section id into zone z at index i (clamped).
func (l *Layout) Move(id string, z, i int) {
	if oz, oi := l.Find(id); oz >= 0 {
		list := l.Zone(oz)
		*list = append((*list)[:oi:oi], (*list)[oi+1:]...)
	}
	list := l.Zone(z)
	i = clamp(i, 0, len(*list))
	*list = append((*list)[:i:i], append([]string{id}, (*list)[i:]...)...)
}

func (l *Layout) normalize() {
	seen := map[string]bool{}
	for z := range ZoneCount {
		list := l.Zone(z)
		out := make([]string, 0, len(*list))
		for _, s := range *list {
			if seen[s] || !validSection(s) {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
		*list = out
	}
	for _, s := range Sections {
		if !seen[s] {
			l.Right = append(l.Right, s)
		}
	}
	l.LeftOffset = clamp(l.LeftOffset, 0, 2000)
	l.CenterOffset = clamp(l.CenterOffset, -2000, 2000)
	l.RightOffset = clamp(l.RightOffset, 0, 2000)
}

func validSection(s string) bool {
	for _, v := range Sections {
		if v == s {
			return true
		}
	}
	return false
}

// Visualizer configures the audio spectrum bars. Defaults match FluentFlyout.
type Visualizer struct {
	Enabled bool `json:"enabled"`
	// Bars is the number of bars, 1..20.
	Bars int `json:"bars"`
	// Centered grows bars from the middle instead of the bottom.
	Centered bool `json:"centered"`
	// Baseline keeps a thin bar visible during silence.
	Baseline bool `json:"baseline"`
	// BaselineAutoHide hides the baseline too when there is no audio.
	BaselineAutoHide bool `json:"baselineAutoHide"`
	// Sensitivity 1..3 lowers the floor: minDb = -10*s - 30.
	Sensitivity int `json:"sensitivity"`
	// Peak 1..3 raises the ceiling: maxDb = 10*p - 30.
	Peak int `json:"peak"`
	// Color is "#RRGGBB". Empty uses the album art color (see AlbumColor),
	// else the Windows accent color.
	Color string `json:"color"`
	// AlbumColor colors the bars with the dominant color of the current
	// album art, like FluentFlyout.
	AlbumColor bool `json:"albumColor"`
}

// Network configures the throughput monitor.
type Network struct {
	Enabled bool `json:"enabled"`
	// Adapter is an interface alias ("Ethernet") or GUID. Empty picks automatically.
	Adapter string `json:"adapter"`
	// Bits shows Mb/s (decimal) instead of MB/s (binary).
	Bits bool `json:"bits"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Visible: true,
		Layout: Layout{
			Center:      []string{SectionVisualizer, SectionMedia},
			Right:       []string{SectionNetwork},
			LeftOffset:  12,
			RightOffset: 12,
		},
		Visualizer: Visualizer{
			Enabled: true, Bars: 10, Sensitivity: 2, Peak: 3, AlbumColor: true,
		},
		Media:   Media{Enabled: true, Controls: true},
		Network: Network{Enabled: true},
	}
}

// Path returns the config file location.
func Path() (string, error) {
	dir, err := os.UserConfigDir() // %APPDATA% on Windows
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "wintoolbar", "config.json"), nil
}

// Load reads the config file. A missing file yields defaults and is written
// so the user has something to edit. A broken file yields defaults and the error.
func Load() (Config, error) {
	c := Default()
	p, err := Path()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return c, Save(c)
	}
	if err != nil {
		return c, err
	}
	// Zone lists present in the file replace the defaults entirely.
	c.Layout.Left, c.Layout.Center, c.Layout.Right = nil, nil, nil
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), err
	}
	if c.Layout.Left == nil && c.Layout.Center == nil && c.Layout.Right == nil {
		d := Default().Layout
		c.Layout.Left, c.Layout.Center, c.Layout.Right = d.Left, d.Center, d.Right
	}
	c.normalize()
	return c, nil
}

func (c *Config) normalize() {
	c.Layout.normalize()
	for device, l := range c.Layouts {
		if device == "" || l == nil {
			delete(c.Layouts, device) // the primary taskbar uses Layout
			continue
		}
		l.normalize()
	}
	v := &c.Visualizer
	v.Bars = clamp(v.Bars, 1, 20)
	v.Sensitivity = clamp(v.Sensitivity, 1, 3)
	v.Peak = clamp(v.Peak, 1, 3)
}

// Save writes the config atomically (temp file, then rename).
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }
