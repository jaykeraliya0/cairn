// Package locale enumerates the timezone, locale and console-keymap choices
// available on the live system.
//
// Everything is read from the files the live ISO already carries rather than
// from a baked-in list, so the choices track whatever tzdata, glibc and kbd
// versions the ISO was built with.
package locale

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Default paths on a live Arch system.
const (
	ZoneinfoDir  = "/usr/share/zoneinfo"
	LocaleGenTpl = "/etc/locale.gen"
	KeymapsDir   = "/usr/share/kbd/keymaps"
)

// nonZoneEntries are the data and index files that live alongside the real
// zones under /usr/share/zoneinfo and must not be offered as timezones.
var nonZoneEntries = map[string]bool{
	"posix": true, "right": true, "posixrules": true, "localtime": true,
	"leapseconds": true, "leap-seconds.list": true, "tzdata.zi": true,
	"iso3166.tab": true, "zone.tab": true, "zone1970.tab": true,
	"zonenow.tab": true, "SECURITY": true, "Factory": true,
}

// Timezones returns every selectable zone under root, as "Region/City" paths,
// sorted. Deprecated single-word aliases (EST, GMT) are dropped in favour of
// the region-qualified names.
func Timezones(root string) ([]string, error) {
	var zones []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}

		// Skip the alternate trees wholesale rather than per-file.
		if top, _, _ := strings.Cut(rel, string(filepath.Separator)); nonZoneEntries[top] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || nonZoneEntries[d.Name()] {
			return nil
		}
		// Real zones are region-qualified and carry no file extension; this
		// drops the tab/list files and the bare "EST"-style compatibility
		// aliases in one test. UTC is the exception worth keeping: it is the
		// zone the live ISO itself runs on, and the one people reach for.
		if strings.Contains(d.Name(), ".") {
			return nil
		}
		if !strings.Contains(rel, string(filepath.Separator)) && rel != "UTC" {
			return nil
		}
		zones = append(zones, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading timezones from %s: %w", root, err)
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("no timezones found under %s", root)
	}

	sort.Strings(zones)
	return zones, nil
}

// Locale is one entry from /etc/locale.gen.
type Locale struct {
	// Name is the locale itself, e.g. "en_US.UTF-8" — what LANG is set to.
	Name string
	// GenLine is the whole locale.gen line, e.g. "en_US.UTF-8 UTF-8" — what
	// locale-gen needs uncommented to actually build the locale.
	GenLine string
	// Label is what the picker shows. Ordered() fills it in with a readable
	// name for the common locales and the bare name for the rest.
	Label string
}

// Locales returns the UTF-8 locales listed in a locale.gen template, sorted.
//
// The file ships with every locale present but commented out, which is exactly
// the catalogue we want. Non-UTF-8 charsets are filtered: they would be a
// regression to offer on a new install, and they triple the length of the list.
func Locales(path string) ([]Locale, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading locales from %s: %w", path, err)
	}
	defer f.Close()

	seen := map[string]bool{}
	var locales []Locale
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "#"))
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.EqualFold(fields[1], "UTF-8") {
			continue
		}
		if seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		locales = append(locales, Locale{Name: fields[0], GenLine: fields[0] + " " + fields[1]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading locales from %s: %w", path, err)
	}
	if len(locales) == 0 {
		return nil, fmt.Errorf("no UTF-8 locales found in %s", path)
	}

	sort.Slice(locales, func(i, j int) bool { return locales[i].Name < locales[j].Name })
	return locales, nil
}

// Keymaps returns the console keymap names under root, sorted and de-duplicated.
// These are the names `loadkeys` and /etc/vconsole.conf's KEYMAP= accept.
func Keymaps(root string) ([]string, error) {
	seen := map[string]bool{}
	var keymaps []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A missing sub-tree is not worth failing the whole scan over.
			return nil //nolint:nilerr // best-effort walk
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		// Files are <name>.map.gz, occasionally <name>.map. Includes live in
		// an "include" subdirectory and are not selectable on their own.
		base, ok := strings.CutSuffix(name, ".map.gz")
		if !ok {
			if base, ok = strings.CutSuffix(name, ".map"); !ok {
				return nil
			}
		}
		if strings.Contains(path, string(filepath.Separator)+"include"+string(filepath.Separator)) {
			return nil
		}
		if !seen[base] {
			seen[base] = true
			keymaps = append(keymaps, base)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading keymaps from %s: %w", root, err)
	}
	if len(keymaps) == 0 {
		return nil, fmt.Errorf("no keymaps found under %s", root)
	}

	sort.Strings(keymaps)
	return keymaps, nil
}
