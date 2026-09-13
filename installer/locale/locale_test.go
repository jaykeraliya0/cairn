package locale

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeZoneinfo builds a directory shaped like /usr/share/zoneinfo: real zones,
// the posix/right duplicate trees, the index tables, and the bare
// compatibility aliases.
func fakeZoneinfo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := []string{
		"Europe/London", "Europe/Paris", "Asia/Kolkata", "America/New_York",
		"Australia/Sydney", "Etc/UTC", "UTC", "EST", "GMT",
		"posix/Europe/London", "right/Europe/London",
		"zone.tab", "zone1970.tab", "iso3166.tab", "leapseconds", "tzdata.zi",
	}
	for _, f := range files {
		path := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("TZif"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTimezones(t *testing.T) {
	zones, err := Timezones(fakeZoneinfo(t))
	if err != nil {
		t.Fatalf("Timezones: %v", err)
	}

	// UTC survives the alias filter; EST and GMT do not.
	want := []string{"America/New_York", "Asia/Kolkata", "Australia/Sydney", "Etc/UTC", "Europe/London", "Europe/Paris", "UTC"}
	if len(zones) != len(want) {
		t.Fatalf("Timezones = %v, want %v", zones, want)
	}
	for i := range want {
		if zones[i] != want[i] {
			t.Fatalf("Timezones = %v, want %v (sorted)", zones, want)
		}
	}

	// The posix/ and right/ trees hold duplicate copies of every zone; offering
	// them would triple the list with entries nobody wants.
	for _, zone := range zones {
		if len(zone) > 6 && (zone[:6] == "posix/" || zone[:6] == "right/") {
			t.Errorf("Timezones included the duplicate tree entry %q", zone)
		}
	}
}

func TestTimezonesMissingDirectory(t *testing.T) {
	if _, err := Timezones(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("Timezones on a missing directory = nil error, want an error")
	}
}

func TestLocales(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locale.gen")
	body := `# Configuration file for locale-gen
#
#en_US.UTF-8 UTF-8
#en_US ISO-8859-1
#en_GB.UTF-8 UTF-8
#de_DE.UTF-8 UTF-8
#de_DE@euro ISO-8859-15
#ja_JP.UTF-8 UTF-8
en_US.UTF-8 UTF-8
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	locales, err := Locales(path)
	if err != nil {
		t.Fatalf("Locales: %v", err)
	}

	// Non-UTF-8 charsets are dropped, and the already-enabled en_US.UTF-8 line
	// must not produce a duplicate of the commented one.
	want := []string{"de_DE.UTF-8", "en_GB.UTF-8", "en_US.UTF-8", "ja_JP.UTF-8"}
	if len(locales) != len(want) {
		t.Fatalf("Locales returned %d entries, want %d: %+v", len(locales), len(want), locales)
	}
	for i, name := range want {
		if locales[i].Name != name {
			t.Fatalf("Locales[%d].Name = %q, want %q", i, locales[i].Name, name)
		}
		if locales[i].GenLine != name+" UTF-8" {
			t.Errorf("Locales[%d].GenLine = %q, want %q", i, locales[i].GenLine, name+" UTF-8")
		}
	}
}

func TestKeymaps(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"i386/qwerty/us.map.gz",
		"i386/qwerty/uk.map.gz",
		"i386/qwertz/de-latin1.map.gz",
		"i386/azerty/fr.map.gz",
		"i386/include/linux-keys-bare.inc.gz",
		"i386/qwerty/../include/qwerty-layout.inc",
		"amiga/amiga-us.map.gz",
		"README.txt",
	}
	for _, f := range files {
		path := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	keymaps, err := Keymaps(root)
	if err != nil {
		t.Fatalf("Keymaps: %v", err)
	}

	want := map[string]bool{"us": true, "uk": true, "de-latin1": true, "fr": true, "amiga-us": true}
	if len(keymaps) != len(want) {
		t.Fatalf("Keymaps = %v, want exactly %v", keymaps, want)
	}
	for _, keymap := range keymaps {
		if !want[keymap] {
			t.Errorf("Keymaps included %q, which is not a selectable keymap", keymap)
		}
	}
}
