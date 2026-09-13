package locale

import (
	"strings"
	"testing"
)

func TestCommonLayoutsAreWellFormed(t *testing.T) {
	seenKeymap := map[string]bool{}
	seenLabel := map[string]bool{}

	for _, layout := range commonLayouts {
		switch {
		case layout.Label == "":
			t.Errorf("%q has no label", layout.Keymap)
		case layout.Keymap == "":
			t.Errorf("%q has no keymap", layout.Label)
		case layout.XKB == "":
			// Without this Hyprland would fall back to us and the installed
			// desktop would not match the console.
			t.Errorf("%q has no XKB layout", layout.Label)
		case seenKeymap[layout.Keymap]:
			t.Errorf("keymap %q appears twice", layout.Keymap)
		case seenLabel[layout.Label]:
			t.Errorf("label %q appears twice", layout.Label)
		}
		seenKeymap[layout.Keymap] = true
		seenLabel[layout.Label] = true
	}

	// English variants have to lead, or the default is buried on a later page
	// of a list whose other entries nobody scanning for "English" will read.
	for i := 0; i < 4; i++ {
		if !strings.HasPrefix(commonLayouts[i].Label, "English") {
			t.Fatalf("commonLayouts[%d] is %q, expected the English variants first",
				i, commonLayouts[i].Label)
		}
	}
	if commonLayouts[0].Keymap != "us" {
		t.Errorf("the list opens on %q, expected us", commonLayouts[0].Keymap)
	}
}

func TestLayouts(t *testing.T) {
	system := []string{"us", "de-latin1", "amiga-us", "applkey", "fr-latin1", "zz-made-up"}

	layouts := Layouts(system)
	if len(layouts) != len(system) {
		t.Fatalf("Layouts returned %d entries for %d keymaps: %+v", len(layouts), len(system), layouts)
	}

	// Curated entries come first, in curated order, with their real labels.
	if layouts[0].Label != "English (US)" || layouts[0].XKB != "us" {
		t.Errorf("first entry = %+v, want English (US)/us", layouts[0])
	}
	// Then the rest of the curated list in its own order, which is alphabetical
	// by label — so French precedes German.
	if layouts[1].Label != "French" || layouts[1].XKB != "fr" {
		t.Errorf("second entry = %+v, want French/fr", layouts[1])
	}
	if layouts[2].Label != "German" || layouts[2].XKB != "de" {
		t.Errorf("third entry = %+v, want German/de", layouts[2])
	}

	// The uncurated tail follows, alphabetically, labelled by its own name and
	// with a derived XKB layout.
	tail := layouts[3:]
	want := []string{"amiga-us", "applkey", "zz-made-up"}
	for i, label := range want {
		if tail[i].Label != label {
			t.Errorf("tail[%d] = %q, want %q", i, tail[i].Label, label)
		}
		if tail[i].XKB == "" {
			t.Errorf("tail entry %q has no XKB layout", label)
		}
	}
}

func TestLayoutsDropsKeymapsTheSystemLacks(t *testing.T) {
	// A curated entry for a keymap kbd no longer ships must not be offered:
	// choosing it would write a vconsole.conf that loadkeys cannot satisfy.
	layouts := Layouts([]string{"us"})
	if len(layouts) != 1 || layouts[0].Keymap != "us" {
		t.Fatalf("Layouts = %+v, want only the us entry", layouts)
	}
}

func TestLayoutsWithNoSystemList(t *testing.T) {
	// An empty system list means the scan failed, not that no keymap exists;
	// offering the curated set beats offering nothing.
	if got := Layouts(nil); len(got) != len(commonLayouts) {
		t.Errorf("Layouts(nil) returned %d entries, want the %d curated ones", len(got), len(commonLayouts))
	}
}

func TestOrdered(t *testing.T) {
	in := []Locale{
		{Name: "aa_DJ.UTF-8", GenLine: "aa_DJ.UTF-8 UTF-8"},
		{Name: "de_DE.UTF-8", GenLine: "de_DE.UTF-8 UTF-8"},
		{Name: "en_GB.UTF-8", GenLine: "en_GB.UTF-8 UTF-8"},
		{Name: "en_US.UTF-8", GenLine: "en_US.UTF-8 UTF-8"},
		{Name: "zu_ZA.UTF-8", GenLine: "zu_ZA.UTF-8 UTF-8"},
	}

	got := Ordered(in)
	if len(got) != len(in) {
		t.Fatalf("Ordered returned %d entries, want %d", len(got), len(in))
	}

	// English first, then the other labelled locales, then the rest.
	wantOrder := []string{"en_GB.UTF-8", "en_US.UTF-8", "de_DE.UTF-8", "aa_DJ.UTF-8", "zu_ZA.UTF-8"}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Fatalf("Ordered[%d] = %q, want %q (full: %+v)", i, got[i].Name, name, got)
		}
	}

	if got[1].Label != "English (United States)" {
		t.Errorf("en_US label = %q, want the readable name", got[1].Label)
	}
	// An uncurated locale still needs a label, or the picker shows a blank row.
	if got[3].Label != "aa_DJ.UTF-8" {
		t.Errorf("uncurated label = %q, want the bare name", got[3].Label)
	}
	// GenLine has to survive the reordering: it is what locale-gen needs.
	if got[1].GenLine != "en_US.UTF-8 UTF-8" {
		t.Errorf("GenLine = %q, want it preserved", got[1].GenLine)
	}
}
