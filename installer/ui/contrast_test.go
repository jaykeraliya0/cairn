package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Contrast floors, from WCAG 2.1.
//
// 4.5:1 is the AA threshold for normal-size text. Everything in this interface
// is normal-size text — there is no large type — so anything carrying words is
// held to it. The decorative floor is far lower because a box edge that is
// merely visible is doing its job.
const (
	textContrast       = 4.5
	decorativeContrast = 1.5
)

// relativeLuminance implements the WCAG definition for an #rrggbb colour.
func relativeLuminance(hex string) float64 {
	hex = strings.TrimPrefix(hex, "#")

	channel := func(i int) float64 {
		v, err := strconv.ParseUint(hex[i:i+2], 16, 8)
		if err != nil {
			return 0
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(0) + 0.7152*channel(2) + 0.0722*channel(4)
}

// contrastRatio is the WCAG ratio between two colours, from 1:1 to 21:1.
func contrastRatio(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestPaletteIsReadable is the test this palette needed from the start.
//
// An earlier version drew every hint, placeholder, status line and the
// progress bar's track in colors.lua's `bg_hl` — a background shade, at
// 1.3:1. On screen that is not "subtle", it is blank. Nothing catches that but
// looking at a real console, so the floor lives here instead.
func TestPaletteIsReadable(t *testing.T) {
	bg := string(colorBg)

	text := map[string]lipgloss.Color{
		"colorFg":     colorFg,
		"colorFgDim":  colorFgDim,
		"colorMuted":  colorMuted,
		"colorAccent": colorAccent,
		"colorGreen":  colorGreen,
		"colorRed":    colorRed,
	}
	for name, colour := range text {
		if got := contrastRatio(string(colour), bg); got < textContrast {
			t.Errorf("%s (%s) is %.2f:1 against the background, want at least %.1f:1",
				name, colour, got, textContrast)
		}
	}

	// Decorative marks may be quiet, but never invisible.
	if got := contrastRatio(string(colorBorder), bg); got < decorativeContrast {
		t.Errorf("colorBorder (%s) is %.2f:1, want at least %.1f:1 to be visible at all",
			colorBorder, got, decorativeContrast)
	}
	// ...and never so loud they compete with the words.
	if got := contrastRatio(string(colorBorder), bg); got > textContrast {
		t.Errorf("colorBorder (%s) is %.2f:1, loud enough to compete with text", colorBorder, got)
	}

	// The tiers have to stay distinguishable from each other, or the hierarchy
	// they encode is decoration.
	ramp := []struct {
		name   string
		colour lipgloss.Color
	}{
		{"colorFg", colorFg}, {"colorFgDim", colorFgDim},
		{"colorMuted", colorMuted}, {"colorBorder", colorBorder},
	}
	for i := 1; i < len(ramp); i++ {
		brighter := relativeLuminance(string(ramp[i-1].colour))
		dimmer := relativeLuminance(string(ramp[i].colour))
		if dimmer >= brighter {
			t.Errorf("%s is not dimmer than %s", ramp[i].name, ramp[i-1].name)
		}
	}
}

// TestVTPaletteIsReadable holds the console fallback to the same floor.
//
// The sixteen slots are what lipgloss degrades to on a virtual console, so a
// dim tier that is readable in truecolor and invisible on the VT would move
// the bug rather than fix it. Slot 8 is the one that matters: terminals reach
// for "bright black" when asked to dim text.
func TestVTPaletteIsReadable(t *testing.T) {
	bg := vtPalette[0]

	for _, slot := range []int{1, 2, 3, 6, 7, 8, 9, 10, 11, 14, 15} {
		if got := contrastRatio(vtPalette[slot], bg); got < textContrast {
			t.Errorf("VT slot %d (#%s) is %.2f:1 against slot 0, want at least %.1f:1",
				slot, vtPalette[slot], got, textContrast)
		}
	}

	// The truecolor palette and the console one describe the same theme; if
	// they drift, the installer looks like a different program on a VT.
	pairs := map[string]lipgloss.Color{
		"3": colorAccent, "6": colorFgDim, "7": colorFg, "8": colorMuted,
		"1": colorRed, "2": colorGreen,
	}
	for slot, colour := range pairs {
		index, _ := strconv.Atoi(slot)
		if want := strings.TrimPrefix(string(colour), "#"); vtPalette[index] != want {
			t.Errorf("VT slot %s is #%s, want #%s to match the truecolor palette",
				slot, vtPalette[index], want)
		}
	}
}
